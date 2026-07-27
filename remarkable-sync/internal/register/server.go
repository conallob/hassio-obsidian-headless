// Package register provides a small HTTP server that guides the user through
// reMarkable device registration without leaving the browser.
//
// Flow:
//  1. User opens the add-on's registration page (e.g. http://homeassistant:8421/).
//  2. The page links to https://my.remarkable.com/device/desktop/new where
//     reMarkable shows an 8-character alphanumeric one-time code (e.g. "bufjmbgl").
//  3. User enters the code into the form and submits.
//  4. The server pipes the code to the rmapi CLI, which completes device
//     registration and persists its own device/user tokens to its config
//     file (RMAPI_CONFIG). We don't handle tokens ourselves at all.
//  5. The confirmation page tells the user registration is done — they can
//     restart the add-on and remove the code from the UI.
package register

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"remarkable-sync/internal/rmapicli"
)

// Server is the registration HTTP server.
type Server struct {
	mux *http.ServeMux
}

// New creates a registration Server.
func New() *Server {
	s := &Server{mux: http.NewServeMux()}
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
  ✓ This device is already registered. The sync service is using it.
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
      After successful registration the sync service picks it up automatically.
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
  rmapi has saved its device credentials. The sync service will use them on
  the next restart.
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
		AlreadyRegistered: rmapicli.IsAuthenticated(r.Context()),
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := rmapicli.Register(ctx, code); err != nil {
		renderError(w, fmt.Sprintf("Registration failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = successTmpl.Execute(w, nil)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	registered := rmapicli.IsAuthenticated(r.Context())
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"registered":%v}`, registered)
}

func renderError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = errorTmpl.Execute(w, struct{ Error string }{Error: msg})
}
