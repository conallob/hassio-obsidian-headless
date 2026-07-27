// Package rmcloud implements a minimal client for the reMarkable cloud API.
// Authentication uses a device token obtained by registering a device via the
// reMarkable desktop/mobile app or the rmapi CLI tool.
package rmcloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// authURL exchanges a device token for a short-lived user token.
	// reMarkable moved this API off my.remarkable.com (which now 405s/redirects
	// to a dead host) to webapp-prod.cloud.remarkable.engineering — see
	// ddvk/rmapi commit b41e13a ("fix auth url", 2022), which we're pinned to
	// (v0.0.34) but had not mirrored in our own independent Go client.
	authURL = "https://webapp-prod.cloud.remarkable.engineering/token/json/2/user/new"
	// docsBase is the document storage service root.
	docsBase = "https://document-storage-production-dot-remarkable-production.appspot.com"
)

// Document represents a reMarkable cloud document or collection.
type Document struct {
	ID                string    `json:"ID"`
	Version           int       `json:"Version"`
	BlobURLGet        string    `json:"BlobURLGet"`
	BlobURLGetExpires string    `json:"BlobURLGetExpires"`
	ModifiedClient    time.Time `json:"ModifiedClient"`
	// Type is either "CollectionType" (folder) or "DocumentType" (notebook/PDF).
	Type         string `json:"Type"`
	VissibleName string `json:"VissibleName"`
	Parent       string `json:"Parent"`
	Bookmarked   bool   `json:"Bookmarked"`
}

// IsCollection returns true for folder entries.
func (d Document) IsCollection() bool { return d.Type == "CollectionType" }

// Client is an authenticated reMarkable cloud client.
type Client struct {
	deviceToken string
	userToken   string
	http        *http.Client
}

// New creates a Client with the given device token.
// Call Authenticate before any other method.
func New(deviceToken string) *Client {
	return &Client{
		deviceToken: deviceToken,
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Don't silently follow redirects: if reMarkable moves an API host
			// again, a followed redirect can land on an unrelated or dead
			// domain, surfacing as a confusing DNS failure instead of a clear
			// non-2xx status from the endpoint we actually intended to call.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Authenticate exchanges the device token for a user token.
func (c *Client) Authenticate() error {
	req, err := http.NewRequest(http.MethodPost, authURL, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.deviceToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed (HTTP %d): %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	c.userToken = string(bytes.TrimSpace(body))
	return nil
}

// ListDocuments returns all documents and collections.
// Pass withBlob=true to include pre-signed download URLs.
func (c *Client) ListDocuments(withBlob bool) ([]Document, error) {
	url := docsBase + "/document-storage/json/2/docs"
	if withBlob {
		url += "?withBlob=true"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.userToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list documents (HTTP %d): %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	var docs []Document
	if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
		return nil, fmt.Errorf("decode document list: %w", err)
	}
	return docs, nil
}

// DownloadBlob fetches the raw ZIP blob for a document from its pre-signed URL.
func (c *Client) DownloadBlob(blobURL string) ([]byte, error) {
	resp, err := c.http.Get(blobURL)
	if err != nil {
		return nil, fmt.Errorf("download blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download blob (HTTP %d)", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// BuildPathMap returns a map of document ID → full display path, built by
// walking parent references. Root-level documents have path == VissibleName.
func BuildPathMap(docs []Document) map[string]string {
	byID := make(map[string]Document, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	cache := make(map[string]string, len(docs))
	var resolve func(id string) string
	resolve = func(id string) string {
		if p, ok := cache[id]; ok {
			return p
		}
		d, ok := byID[id]
		if !ok {
			return ""
		}
		if d.Parent == "" || d.Parent == "trash" {
			cache[id] = d.VissibleName
		} else {
			parent := resolve(d.Parent)
			if parent == "" {
				cache[id] = d.VissibleName
			} else {
				cache[id] = parent + "/" + d.VissibleName
			}
		}
		return cache[id]
	}
	for _, d := range docs {
		resolve(d.ID)
	}
	return cache
}
