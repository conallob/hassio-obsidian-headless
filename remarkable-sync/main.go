// remarkable-sync syncs documents from the reMarkable cloud into an Obsidian
// vault as markdown notes, preserving the full folder hierarchy.
//
// For each document the sync engine:
//  1. Downloads the raw ZIP blob from reMarkable cloud storage.
//  2. Extracts metadata / content JSON from the ZIP to build rich front-matter.
//  3. Saves embedded PDFs (uploaded documents) alongside the markdown.
//  4. Optionally sends rendered page images through a Home Assistant OCR
//     endpoint to transcribe handwritten notebooks.
//
// A cache directory (/data/remarkable-sync/) stores the raw blobs so future
// runs only re-download documents whose cloud version has changed.
//
// Configuration via environment variables (set by the HA supervisor):
//
//	REMARKABLE_DEVICE_TOKEN  reMarkable device token (required)
//	VAULT_PATH               Obsidian vault root (required)
//	REMARKABLE_OUTPUT_DIR    sub-directory within vault (default: reMarkable)
//	REMARKABLE_CACHE_DIR     raw blob cache (default: /data/remarkable-sync)
//	HA_OCR_URL               HA OCR endpoint URL (optional)
//	HA_OCR_TOKEN             HA long-lived access token (optional)
//	HA_OCR_ENTITY            HA entity_id for image_processing (optional)
//	SYNC_INTERVAL            seconds between syncs in continuous mode (default: 300)
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"remarkable-sync/internal/obsidianauth"
	"remarkable-sync/internal/ocr"
	"remarkable-sync/internal/register"
	"remarkable-sync/internal/rmcloud"
)

func main() {
	deviceToken := flag.String("token", env("REMARKABLE_DEVICE_TOKEN", ""), "reMarkable device token (falls back to saved token file)")
	vaultPath := flag.String("vault-path", env("VAULT_PATH", "/share/obsidian-vault"), "Obsidian vault root path")
	outputDir := flag.String("output-dir", env("REMARKABLE_OUTPUT_DIR", "reMarkable"), "sub-directory within vault for reMarkable notes")
	cacheDir := flag.String("cache-dir", env("REMARKABLE_CACHE_DIR", "/data/remarkable-sync"), "directory for raw blob cache and saved token")
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

	// Ensure cache dir exists before we try to read/write the token file.
	if err := os.MkdirAll(*cacheDir, 0o755); err != nil {
		log.Fatalf("cannot create cache dir %s: %v", *cacheDir, err)
	}

	tokenFilePath := filepath.Join(*cacheDir, "device.token")

	// Always start the reMarkable registration web UI.
	regServer := register.New(tokenFilePath)
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

	// Resolve token: explicit flag/env takes priority, then saved file.
	if *deviceToken == "" {
		*deviceToken = register.ReadSavedToken(tokenFilePath)
	}
	if *deviceToken == "" {
		log.Printf("No reMarkable device token configured. Open http://<ha-host>:%s/ to register.", *registerPort)
		// Block until a token is saved via the registration UI, then proceed.
		for *deviceToken == "" {
			time.Sleep(5 * time.Second)
			*deviceToken = register.ReadSavedToken(tokenFilePath)
		}
		log.Println("Device token received — starting sync")
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
			if err := runSync(*deviceToken, destRoot, *cacheDir, ocrClient); err != nil {
				log.Printf("sync error: %v", err)
			}
			time.Sleep(time.Duration(*intervalSec) * time.Second)
		}
	} else {
		if err := runSync(*deviceToken, destRoot, *cacheDir, ocrClient); err != nil {
			log.Fatalf("sync failed: %v", err)
		}
	}
}

func runSync(deviceToken, destRoot, cacheDir string, ocrClient *ocr.Client) error {
	client := rmcloud.New(deviceToken)
	if err := client.Authenticate(); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	docs, err := client.ListDocuments(true)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}

	paths := rmcloud.BuildPathMap(docs)

	var synced int
	for _, doc := range docs {
		if doc.IsCollection() {
			continue
		}
		docPath := paths[doc.ID]
		changed, err := syncDocument(client, doc, docPath, destRoot, cacheDir, ocrClient)
		if err != nil {
			log.Printf("skip %q: %v", doc.VissibleName, err)
		} else if changed {
			synced++
		}
	}

	if synced > 0 {
		// Write an index file at the vault output root listing all documents.
		if err := writeIndex(destRoot, docs, paths); err != nil {
			log.Printf("index write error: %v", err)
		}
		log.Printf("Sync complete: %d document(s) updated", synced)
	}

	return nil
}

// syncDocument writes (or updates) the markdown note and cached blob for a document.
// Returns true if the document was downloaded and written (i.e. the cloud version changed).
func syncDocument(client *rmcloud.Client, doc rmcloud.Document, docPath, destRoot, cacheDir string, ocrClient *ocr.Client) (bool, error) {
	notePath := filepath.Join(destRoot, docPath+".md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		return false, err
	}

	// Use a version stamp file so we only re-download when the cloud version changes.
	stampPath := filepath.Join(cacheDir, doc.ID+".v")
	currentStamp := fmt.Sprintf("%d", doc.Version)
	if existing, err := os.ReadFile(stampPath); err == nil && string(existing) == currentStamp {
		return false, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return false, err
	}

	var blob []byte
	var blobMeta blobMetadata
	if doc.BlobURLGet != "" {
		var err error
		blob, err = client.DownloadBlob(doc.BlobURLGet)
		if err != nil {
			log.Printf("could not download blob for %q: %v — writing stub note", doc.VissibleName, err)
		} else {
			blobMeta = extractBlobMetadata(blob)
			// Save embedded PDF if present.
			if len(blobMeta.PDF) > 0 {
				pdfPath := filepath.Join(destRoot, docPath+".pdf")
				if err := os.WriteFile(pdfPath, blobMeta.PDF, 0o644); err != nil {
					log.Printf("could not save PDF for %q: %v", doc.VissibleName, err)
				}
			}
		}
	}

	var ocrText string
	if ocrClient != nil && blob != nil {
		ocrText = extractOCRText(blob, doc, ocrClient)
	}

	if err := writeNote(notePath, doc, docPath, blobMeta, ocrText); err != nil {
		return false, err
	}

	// Write version stamp only after a successful note write.
	return true, os.WriteFile(stampPath, []byte(currentStamp), 0o644)
}

// blobMetadata holds structured data extracted from a reMarkable ZIP blob.
type blobMetadata struct {
	// PageCount from the .content JSON.
	PageCount int
	// Tags from the .content JSON.
	Tags []string
	// FileType: "notebook", "pdf", "epub".
	FileType string
	// PDF contains the raw bytes of an embedded PDF document (may be nil).
	PDF []byte
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

// extractBlobMetadata parses the ZIP blob and pulls structured metadata out.
func extractBlobMetadata(blob []byte) blobMetadata {
	r, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return blobMetadata{}
	}

	var meta blobMetadata
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(name))

		switch {
		case ext == ".content":
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

		case ext == ".pdf":
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

// extractOCRText sends PNG/JPEG page images from the blob through the OCR client.
func extractOCRText(blob []byte, doc rmcloud.Document, ocrClient *ocr.Client) string {
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
			log.Printf("OCR error for page %s in %q: %v", f.Name, doc.VissibleName, err)
			continue
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// writeNote renders the markdown file for a document.
func writeNote(path string, doc rmcloud.Document, docPath string, meta blobMetadata, ocrText string) error {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", doc.VissibleName))
	sb.WriteString(fmt.Sprintf("remarkable_id: %q\n", doc.ID))
	sb.WriteString(fmt.Sprintf("modified: %s\n", doc.ModifiedClient.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("remarkable_path: %q\n", docPath))
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
	if doc.Bookmarked {
		sb.WriteString("bookmarked: true\n")
	}
	sb.WriteString("source: reMarkable\n")
	sb.WriteString("---\n\n")

	sb.WriteString("# " + doc.VissibleName + "\n\n")

	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString(fmt.Sprintf("| Modified | %s |\n", doc.ModifiedClient.UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString(fmt.Sprintf("| reMarkable ID | `%s` |\n", doc.ID))
	sb.WriteString(fmt.Sprintf("| Version | %d |\n", doc.Version))
	if meta.PageCount > 0 {
		sb.WriteString(fmt.Sprintf("| Pages | %d |\n", meta.PageCount))
	}
	if meta.FileType != "" {
		sb.WriteString(fmt.Sprintf("| Type | %s |\n", meta.FileType))
	}
	if len(meta.PDF) > 0 {
		sb.WriteString(fmt.Sprintf("| PDF | [[%s.pdf]] |\n", filepath.Base(docPath)))
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
func writeIndex(destRoot string, docs []rmcloud.Document, paths map[string]string) error {
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
		if doc.IsCollection() {
			continue
		}
		p := paths[doc.ID]
		sb.WriteString(fmt.Sprintf("| [[%s]] | %s | document |\n",
			p, doc.ModifiedClient.UTC().Format("2006-01-02")))
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
