// Package ocr provides a Home Assistant image-processing OCR client.
// It POSTs image bytes to a HA webhook or rest_command endpoint and returns
// the recognised text.
package ocr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client sends images to a Home Assistant OCR endpoint and returns text.
//
// HA side: create a rest_command or webhook automation that accepts
// {"image": "<base64>"} and returns {"text": "<recognised text>"}.
// The HAURL must be the full URL, e.g.:
//
//	http://homeassistant.local:8123/api/webhook/remarkable-ocr
type Client struct {
	haURL  string
	token  string
	entity string
	http   *http.Client
}

// New creates a Client.
//   - haURL: full URL to the HA OCR endpoint
//   - token: Home Assistant long-lived access token (may be empty for webhook endpoints)
//   - entity: optional entity_id to pass in the payload (for image_processing integrations)
func New(haURL, token, entity string) *Client {
	return &Client{
		haURL:  strings.TrimRight(haURL, "/"),
		token:  token,
		entity: entity,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

type ocrRequest struct {
	Image    string `json:"image"`
	EntityID string `json:"entity_id,omitempty"`
}

type ocrResponse struct {
	Text string `json:"text"`
}

// Recognise sends imageBytes (PNG or JPEG) to the HA endpoint and returns text.
// Returns empty string without error when the endpoint returns no text field.
func (c *Client) Recognise(imageBytes []byte) (string, error) {
	payload := ocrRequest{
		Image:    base64.StdEncoding.EncodeToString(imageBytes),
		EntityID: c.entity,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, c.haURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("OCR request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OCR endpoint (HTTP %d): %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	var result ocrResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		// Non-JSON response — return raw body as text if non-empty.
		if text := strings.TrimSpace(string(raw)); text != "" {
			return text, nil
		}
		return "", nil
	}
	return result.Text, nil
}
