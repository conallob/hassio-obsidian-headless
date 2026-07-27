// Package register provides a small HTTP server that guides the user through
// reMarkable device registration without leaving the browser.
//
// Flow:
//  1. User opens the add-on's registration page (e.g. http://homeassistant:8421/).
//  2. The page links to https://my.remarkable.com/device/desktop/new where
//     reMarkable shows an 8-character alphanumeric one-time code (e.g. "bufjmbgl").
//  3. User enters the code into the form and submits.
//  4. The server calls the reMarkable registration API, receives a device token,
//     and writes it to tokenPath.
//  5. The confirmation page tells the user the token is saved — they can restart
//     the add-on and remove the code from the UI.
package register

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// registrationAPI moved off my.remarkable.com (which now 405s on this route)
// to webapp-prod.cloud.remarkable.engineering — see ddvk/rmapi commit
// b41e13a ("fix auth url", 2022); we're pinned to that fork (v0.0.34) but
// this is our own independent client and hadn't picked up the host change.
const registrationAPI = "https://webapp-prod.cloud.remarkable.engineering/token/json/2/device/new"

// Server is the registration HTTP server.
type Server struct {
	tokenPath string
	mux       *http.ServeMux
}

// New creates a Server that will save the device token to tokenPath.
func New(tokenPath string) *Server {
	s := &Server{tokenPath: tokenPath, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/register", s.handleRegister)
	s.mux.HandleFunc("/status", s.handleStatus)
	return s
}

// ListenAndServe starts the HTTP server on addr (e.g. ":8421").
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return srv.ListenAndServe()
}

// TokenPath returns the path where the device token is saved.
func (s *Server) TokenPath() string { return s.tokenPath }

// ReadSavedToken returns the saved device token, or empty string if not yet registered.
func ReadSavedToken(tokenPath string) string {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>reMarkable Device Registration</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:560px;margin:3rem auto;padding:0 1rem;color:#1a1a1a}
  h1{font-size:1.4rem;margin-bottom:.25rem}
  .subtitle{color:#666;margin-bottom:2rem;font-size:.95rem}
  .step{display:flex;gap:1rem;margin-bottom:1.5rem;align-items:flex-start}
  .step-num{background:#1a1a1a;color:#fff;border-radius:50%;width:28px;height:28px;
    display:flex;align-items:center;justify-content:center;font-size:.85rem;flex-shrink:0;margin-top:2px}
  .step-body{flex:1}
  a{color:#0066cc}
  input[type=text]{border:1px solid #ccc;border-radius:6px;padding:.5rem .75rem;
    font-size:1.1rem;letter-spacing:.15em;width:10ch}
  button{background:#1a1a1a;color:#fff;border:none;border-radius:6px;
    padding:.55rem 1.25rem;font-size:1rem;cursor:pointer;margin-top:.5rem}
  button:hover{background:#333}
  .status{padding:.75rem 1rem;border-radius:6px;margin-top:1.5rem;font-size:.9rem}
  .status.ok{background:#d4edda;color:#155724}
  .status.err{background:#f8d7da;color:#721c24}
  .status.already{background:#d1ecf1;color:#0c5460}
  label{display:block;margin-bottom:.4rem;font-weight:500}
</style>
</head>
<body>
<h1>reMarkable Device Registration</h1>
<p class="subtitle">Connect your reMarkable account to this add-on — no app installation needed.</p>

{{if .AlreadyRegistered}}
<div class="status already">
  ✓ A device token is already saved. The sync service is using it.
  You can re-register below to replace it.
</div>
{{end}}

<div class="step">
  <div class="step-num">1</div>
  <div class="step-body">
    <strong>Get a pairing code from reMarkable</strong><br>
    Open
    <a href="https://my.remarkable.com/device/desktop/new" target="_blank" rel="noopener">
      my.remarkable.com/device/desktop/new ↗
    </a>,
    then click the <strong>Tablet</strong> tab. reMarkable will display an
    8-character code (e.g. <code>xxxxxxxx</code>) valid for ~5 minutes.<br>
    <span style="color:#666;font-size:.9rem">
      The code is lowercase — enter it exactly as shown.
    </span>
  </div>
</div>

<div class="step">
  <div class="step-num">2</div>
  <div class="step-body">
    <strong>Enter the code below</strong>
    <form method="POST" action="/register" style="margin-top:.6rem">
      <label for="code">One-time code</label>
      <input id="code" name="code" type="text" maxlength="8" autocomplete="off"
             placeholder="xxxxxxxx" required>
      <br>
      <button type="submit">Register device</button>
    </form>
  </div>
</div>

<div class="step">
  <div class="step-num">3</div>
  <div class="step-body">
    <strong>Restart the add-on</strong><br>
    <span style="color:#666;font-size:.9rem">
      After successful registration the token is saved automatically.
      Restart the add-on — no config changes needed.
    </span>
  </div>
</div>
</body>
</html>`))

var successTmpl = template.Must(template.New("success").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Registration successful</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:560px;margin:3rem auto;padding:0 1rem;color:#1a1a1a}
  h1{font-size:1.4rem}
  .status{padding:.75rem 1rem;border-radius:6px;font-size:.9rem;background:#d4edda;color:#155724}
  a{color:#0066cc}
</style>
</head>
<body>
<h1>✓ Device registered successfully</h1>
<div class="status">
  The device token has been saved to <code>{{.TokenPath}}</code>.
  The sync service will use it on the next restart.
</div>
<p>You can now <strong>restart the add-on</strong> from the Home Assistant UI.</p>
<p><a href="/">← Back</a></p>
</body>
</html>`))

var errorTmpl = template.Must(template.New("error").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Registration failed</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:560px;margin:3rem auto;padding:0 1rem;color:#1a1a1a}
  h1{font-size:1.4rem}
  .status{padding:.75rem 1rem;border-radius:6px;font-size:.9rem;background:#f8d7da;color:#721c24}
  a{color:#0066cc}
</style>
</head>
<body>
<h1>Registration failed</h1>
<div class="status">{{.Error}}</div>
<p><a href="/">← Try again</a></p>
</body>
</html>`))

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := struct{ AlreadyRegistered bool }{
		AlreadyRegistered: ReadSavedToken(s.tokenPath) != "",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTmpl.Execute(w, data)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, "could not parse form")
		return
	}
	code := strings.TrimSpace(strings.ToLower(r.FormValue("code")))
	if code == "" {
		renderError(w, "one-time code is required")
		return
	}

	token, err := registerDevice(code)
	if err != nil {
		log.Printf("registration failed: %v", err)
		renderError(w, fmt.Sprintf("Registration failed: %v", err))
		return
	}

	if err := saveToken(s.tokenPath, token); err != nil {
		log.Printf("could not save token: %v", err)
		renderError(w, fmt.Sprintf("Could not save token: %v", err))
		return
	}

	log.Printf("Device registered successfully; token saved to %s", s.tokenPath)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = successTmpl.Execute(w, struct{ TokenPath string }{TokenPath: s.tokenPath})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	registered := ReadSavedToken(s.tokenPath) != ""
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"registered":%v}`, registered)
}

type registrationRequest struct {
	Code       string `json:"code"`
	DeviceDesc string `json:"deviceDesc"`
	DeviceID   string `json:"deviceID"`
}

// deviceDesc must match a value reMarkable's backend recognizes.
// "desktop-linux" is what ddvk/rmapi's current (non-legacy) code sends;
// its legacy/dead code path used "desktop-windows", which we'd copied.
const deviceDesc = "desktop-linux"

func registerDevice(code string) (string, error) {
	payload := registrationRequest{
		Code:       code,
		DeviceDesc: deviceDesc,
		DeviceID:   uuid.New().String(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Use a custom client that preserves the POST method on redirects and
	// sends Authorization: Bearer (empty token) as required by the reMarkable API.
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 {
				req.Method = via[0].Method
				req.Header = via[0].Header.Clone()
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodPost, registrationAPI, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer")
	req.Header.Set("User-Agent", "rmapi")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("registration API: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("empty token returned")
	}
	return token, nil
}

func saveToken(tokenPath, token string) error {
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(tokenPath, []byte(token), 0o600)
}

func renderError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = errorTmpl.Execute(w, struct{ Error string }{Error: msg})
}
