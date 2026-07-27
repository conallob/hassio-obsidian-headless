// remarkable-sync syncs documents from the reMarkable cloud into an Obsidian
// vault as markdown notes, preserving the full folder hierarchy.
//
// Device registration and cloud sync are both delegated to the rmapi CLI
// (github.com/ddvk/rmapi, already built into this add-on's image) rather
// than reimplementing reMarkable's private API ourselves — the same pattern
// this add-on uses for obsidian-headless and obsidian-web-mcp. See
// internal/rmapicli.
//
// For each document rmapi fetches, this binary:
//  1. Parses the downloaded .rmdoc ZIP bundle for embedded metadata/PDF.
//  2. Writes a Markdown note with YAML front-matter.
//  3. Saves embedded PDFs (uploaded documents) alongside the markdown.
//  4. Optionally sends rendered page images through a Home Assistant OCR
//     endpoint to transcribe handwritten notebooks.
//
// A cache directory (/data/remarkable-sync/) stores rmapi's local mirror so
// future runs only re-fetch documents whose cloud version has changed.
//
// Configuration via environment variables (set by the HA supervisor):
//
//	VAULT_PATH               Obsidian vault root (required)
//	REMARKABLE_OUTPUT_DIR    sub-directory within vault (default: reMarkable)
//	REMARKABLE_CACHE_DIR     rmapi mirror + stamp cache (default: /data/remarkable-sync)
//	HA_OCR_URL               HA OCR endpoint URL (optional)
//	HA_OCR_TOKEN             HA long-lived access token (optional)
//	HA_OCR_ENTITY            HA entity_id for image_processing (optional)
//	SYNC_INTERVAL            seconds between syncs in continuous mode (default: 300)
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"remarkable-sync/internal/obsidianauth"
	"remarkable-sync/internal/ocr"
	"remarkable-sync/internal/register"
	"remarkable-sync/internal/rmapicli"
)

func main() {
	vaultPath := flag.String("vault-path", env("VAULT_PATH", "/share/obsidian-vault"), "Obsidian vault root path")
	outputDir := flag.String("output-dir", env("REMARKABLE_OUTPUT_DIR", "reMarkable"), "sub-directory within vault for reMarkable notes")
	cacheDir := flag.String("cache-dir", env("REMARKABLE_CACHE_DIR", "/data/remarkable-sync"), "directory for rmapi's local mirror and change-stamp cache")
	haOCRURL := flag.String("ha-ocr-url", env("HA_OCR_URL", ""), "Home Assistant OCR endpoint URL (optional)")
	haOCRToken := flag.String("ha-ocr-token", env("HA_OCR_TOKEN", ""), "Home Assistant long-lived access token")
	haOCREntity := flag.String("ha-ocr-entity", env("HA_OCR_ENTITY", ""), "Home Assistant image_processing entity_id")
	continuous := flag.Bool("continuous", env("REMARKABLE_SYNC_ENABLED", "false") == "true", "run continuously (respects SYNC_INTERVAL)")
	intervalSec := flag.Int("interval", envInt("SYNC_INTERVAL", 300), "seconds between syncs in continuous mode")
	registerPort := flag.String("register-port", env("REMARKABLE_REGISTER_PORT", "8421"), "port for the reMarkable device registration web UI")
	obsidianAuthPort := flag.String("obsidian-auth-port", env("OBSIDIAN_AUTH_PORT", "8422"), "port for the Obsidian token generator web UI")
	flag.Parse()

	if *vaultPath == "" {
		log.Fatal("VAULT_PATH is required")
	}

	if err := os.MkdirAll(*cacheDir, 0o755); err != nil {
		log.Fatalf("cannot create cache dir %s: %v", *cacheDir, err)
	}
	if abs, err := filepath.Abs(*cacheDir); err == nil {
		log.Printf("Cache directory ready: %s", abs)
	} else {
		log.Printf("Cache directory ready: %s", *cacheDir)
	}
	mirrorDir := filepath.Join(*cacheDir, "mirror")
	stampDir := filepath.Join(*cacheDir, "stamps")

	// Always start the reMarkable registration web UI. It shells out to rmapi
	// directly (internal/rmapicli) — we no longer manage any device token
	// ourselves, rmapi persists its own to RMAPI_CONFIG.
	regServer := register.New()
	go func() {
		addr := ":" + *registerPort
		log.Printf("reMarkable registration UI listening on %s", addr)
		if err := regServer.ListenAndServe(addr); err != nil {
			log.Printf("reMarkable registration server error: %v", err)
		}
	}()

	// Always start the Obsidian token generator UI.
	// Token is saved to /data/obsidian.token so obsidian-sync picks it up on restart.
	obsidianTokenPath := "/data/obsidian.token"
	go func() {
		addr := ":" + *obsidianAuthPort
		log.Printf("Obsidian auth UI listening on %s", addr)
		if err := obsidianauth.New(obsidianTokenPath).ListenAndServe(addr); err != nil {
			log.Printf("Obsidian auth server error: %v", err)
		}
	}()

	// If reMarkable sync is disabled, keep the web UIs running but skip the sync loop.
	if env("REMARKABLE_SYNC_ENABLED", "true") == "false" {
		log.Println("reMarkable sync disabled — web UIs running, sync loop skipped")
		log.Printf("  Obsidian token generator: http://<ha-host>:%s/", *obsidianAuthPort)
		log.Printf("  reMarkable registration:  http://<ha-host>:%s/", *registerPort)
		select {} // block forever; s6 supervises the process
	}

	if !rmapicli.IsAuthenticated(context.Background()) {
		log.Printf("reMarkable device not registered. Open http://<ha-host>:%s/ to register.", *registerPort)
		for !rmapicli.IsAuthenticated(context.Background()) {
			time.Sleep(5 * time.Second)
		}
		log.Println("Device registered — starting sync")
	}

	var ocrClient *ocr.Client
	if *haOCRURL != "" {
		ocrClient = ocr.New(*haOCRURL, *haOCRToken, *haOCREntity)
		log.Printf("OCR enabled via %s", *haOCRURL)
	}

	destRoot := filepath.Join(*vaultPath, *outputDir)

	if *continuous {
		log.Printf("Starting continuous sync every %ds", *intervalSec)
		for {
			if err := runSync(mirrorDir, stampDir, destRoot, ocrClient); err != nil {
				log.Printf("sync error: %v", err)
			}
			time.Sleep(time.Duration(*intervalSec) * time.Second)
		}
	} else {
		if err := runSync(mirrorDir, stampDir, destRoot, ocrClient); err != nil {
			log.Fatalf("sync failed: %v", err)
		}
	}
}

// docSummary describes one synced document for the vault-root index.
type docSummary struct {
	RelPath  string // relative to destRoot, no extension
	Title    string
	Modified time.Time
}

func runSync(mirrorDir, stampDir, destRoot string, ocrClient *ocr.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := rmapicli.SyncTree(ctx, "/", mirrorDir); err != nil {
		return fmt.Errorf("rmapi sync: %w", err)
	}

	var docs []docSummary
	var synced int
	err := filepath.WalkDir(mirrorDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".rmdoc" {
			return nil
		}
		relPath, err := filepath.Rel(mirrorDir, path)
		if err != nil {
			return nil
		}
		relPath = strings.TrimSuffix(relPath, filepath.Ext(relPath))

		changed, summary, err := syncDocument(path, relPath, stampDir, destRoot, ocrClient)
		if err != nil {
			log.Printf("skip %q: %v", relPath, err)
			return nil
		}
		docs = append(docs, summary)
		if changed {
			synced++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk mirror: %w", err)
	}

	if synced > 0 {
		if err := writeIndex(destRoot, docs); err != nil {
			log.Printf("index write error: %v", err)
		}
		log.Printf("Sync complete: %d document(s) updated", synced)
	}

	return nil
}

// syncDocument writes (or updates) the markdown note and embedded PDF for a
// single .rmdoc bundle already fetched by rmapi at mirrorPath. Returns true
// if the note was (re)written, i.e. the mirrored file changed since last run.
func syncDocument(mirrorPath, relPath, stampDir, destRoot string, ocrClient *ocr.Client) (bool, docSummary, error) {
	title := filepath.Base(relPath)

	info, err := os.Stat(mirrorPath)
	if err != nil {
		return false, docSummary{}, err
	}
	modified := info.ModTime()

	// rmapi's mget -i only rewrites a mirrored file when the cloud copy is
	// newer, so the mirrored file's own mtime is a reliable, simple change
	// signal — no need for our own version numbering.
	stampPath := filepath.Join(stampDir, relPath+".v")
	currentStamp := fmt.Sprintf("%d", modified.UnixNano())
	summary := docSummary{RelPath: relPath, Title: title, Modified: modified}
	if existing, err := os.ReadFile(stampPath); err == nil && string(existing) == currentStamp {
		return false, summary, nil
	}

	notePath := filepath.Join(destRoot, relPath+".md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		return false, summary, err
	}
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o755); err != nil {
		return false, summary, err
	}

	blob, err := os.ReadFile(mirrorPath)
	if err != nil {
		return false, summary, fmt.Errorf("read mirrored bundle: %w", err)
	}

	meta := extractBlobMetadata(blob)
	// Prefer the bundle's own recorded modified time when present.
	if !meta.Modified.IsZero() {
		modified = meta.Modified
		summary.Modified = modified
	}
	if meta.VisibleName != "" {
		title = meta.VisibleName
		summary.Title = title
	}

	if len(meta.PDF) > 0 {
		pdfPath := filepath.Join(destRoot, relPath+".pdf")
		if err := os.WriteFile(pdfPath, meta.PDF, 0o644); err != nil {
			log.Printf("could not save PDF for %q: %v", title, err)
		}
	}

	var ocrText string
	if ocrClient != nil && len(blob) > 0 {
		ocrText = extractOCRText(blob, title, ocrClient)
	}

	if err := writeNote(notePath, title, relPath, modified, meta, ocrText); err != nil {
		return false, summary, err
	}

	return true, summary, os.WriteFile(stampPath, []byte(currentStamp), 0o644)
}

// blobMetadata holds structured data extracted from a reMarkable .rmdoc bundle.
type blobMetadata struct {
	// PageCount from the .content JSON.
	PageCount int
	// Tags from the .content JSON.
	Tags []string
	// FileType: "notebook", "pdf", "epub".
	FileType string
	// PDF contains the raw bytes of an embedded PDF document (may be nil).
	PDF []byte
	// VisibleName, Modified, Pinned come from the .metadata JSON, if present.
	VisibleName string
	Modified    time.Time
	Pinned      bool
}

// rmContent mirrors the fields we care about in reMarkable's .content JSON.
type rmContent struct {
	FileType  string  `json:"fileType"`
	PageCount int     `json:"pageCount"`
	Tags      []rmTag `json:"tags"`
}

type rmTag struct {
	Name string `json:"name"`
}

// rmMetadata mirrors the fields we care about in reMarkable's .metadata JSON.
type rmMetadata struct {
	VisibleName  string `json:"visibleName"`
	LastModified string `json:"lastModified"` // epoch milliseconds, as a string
	Pinned       bool   `json:"pinned"`
}

// extractBlobMetadata parses the .rmdoc ZIP bundle and pulls structured
// metadata out. Best-effort throughout: missing/unparsable files are ignored
// so a document with an unexpected bundle layout still gets a stub note.
func extractBlobMetadata(blob []byte) blobMetadata {
	r, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return blobMetadata{}
	}

	var meta blobMetadata
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(name))

		switch ext {
		case ".content":
			rc, err := f.Open()
			if err != nil {
				continue
			}
			var c rmContent
			_ = json.NewDecoder(rc).Decode(&c)
			rc.Close()
			meta.PageCount = c.PageCount
			meta.FileType = c.FileType
			for _, t := range c.Tags {
				meta.Tags = append(meta.Tags, t.Name)
			}

		case ".metadata":
			rc, err := f.Open()
			if err != nil {
				continue
			}
			var m rmMetadata
			_ = json.NewDecoder(rc).Decode(&m)
			rc.Close()
			meta.VisibleName = m.VisibleName
			meta.Pinned = m.Pinned
			if ms, err := parseEpochMillis(m.LastModified); err == nil {
				meta.Modified = ms
			}

		case ".pdf":
			rc, err := f.Open()
			if err != nil {
				continue
			}
			pdf, err := io.ReadAll(rc)
			rc.Close()
			if err == nil {
				meta.PDF = pdf
			}
		}
	}
	return meta
}

func parseEpochMillis(s string) (time.Time, error) {
	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}

// extractOCRText sends PNG/JPEG page images from the blob through the OCR client.
func extractOCRText(blob []byte, title string, ocrClient *ocr.Client) string {
	r, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return ""
	}

	var parts []string
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(rc)
		rc.Close()

		text, err := ocrClient.Recognise(buf.Bytes())
		if err != nil {
			log.Printf("OCR error for page %s in %q: %v", f.Name, title, err)
			continue
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// writeNote renders the markdown file for a document.
func writeNote(path, title, relPath string, modified time.Time, meta blobMetadata, ocrText string) error {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", title))
	sb.WriteString(fmt.Sprintf("modified: %s\n", modified.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("remarkable_path: %q\n", relPath))
	if meta.FileType != "" {
		sb.WriteString(fmt.Sprintf("remarkable_type: %q\n", meta.FileType))
	}
	if meta.PageCount > 0 {
		sb.WriteString(fmt.Sprintf("page_count: %d\n", meta.PageCount))
	}
	if len(meta.Tags) > 0 {
		sb.WriteString("tags:\n")
		for _, t := range meta.Tags {
			sb.WriteString(fmt.Sprintf("  - %s\n", t))
		}
	}
	if meta.Pinned {
		sb.WriteString("bookmarked: true\n")
	}
	sb.WriteString("source: reMarkable\n")
	sb.WriteString("---\n\n")

	sb.WriteString("# " + title + "\n\n")

	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString(fmt.Sprintf("| Modified | %s |\n", modified.UTC().Format("2006-01-02 15:04 UTC")))
	if meta.PageCount > 0 {
		sb.WriteString(fmt.Sprintf("| Pages | %d |\n", meta.PageCount))
	}
	if meta.FileType != "" {
		sb.WriteString(fmt.Sprintf("| Type | %s |\n", meta.FileType))
	}
	if len(meta.PDF) > 0 {
		sb.WriteString(fmt.Sprintf("| PDF | [[%s.pdf]] |\n", filepath.Base(relPath)))
	}
	sb.WriteString("\n")

	if ocrText != "" {
		sb.WriteString("## Content (OCR)\n\n")
		sb.WriteString(ocrText)
		sb.WriteString("\n")
	} else if meta.FileType == "notebook" || meta.FileType == "" {
		sb.WriteString("*Handwritten notebook — enable `ha_ocr_url` in add-on options to transcribe pages.*\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// writeIndex creates a top-level index.md listing all reMarkable documents.
func writeIndex(destRoot string, docs []docSummary) error {
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("---\ntitle: reMarkable Index\nsource: reMarkable\n---\n\n")
	sb.WriteString("# reMarkable Index\n\n")
	sb.WriteString(fmt.Sprintf("*Last synced: %s*\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString("| Document | Modified | Type |\n")
	sb.WriteString("|---|---|---|\n")
	for _, doc := range docs {
		sb.WriteString(fmt.Sprintf("| [[%s]] | %s | document |\n",
			doc.RelPath, doc.Modified.UTC().Format("2006-01-02")))
	}
	return os.WriteFile(filepath.Join(destRoot, "index.md"), []byte(sb.String()), 0o644)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
