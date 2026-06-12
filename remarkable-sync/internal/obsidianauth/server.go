// Package obsidianauth provides a browser-based Obsidian account token generator.
//
// It proxies the POST /user/signin call to api.obsidian.md server-side
// (bypassing browser CORS restrictions) and returns the token to the user
// so they can paste it into the add-on configuration.
package obsidianauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const obsidianAPIBase = "https://api.obsidian.md"

// Server is the Obsidian token-generation HTTP server.
type Server struct {
	mux *http.ServeMux
}

// New creates a Server.
func New() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/signin", s.handleSignin)
	return s
}

// ListenAndServe starts the server on addr (e.g. ":8422").
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
<title>Obsidian Auth Token</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:520px;margin:3rem auto;padding:0 1rem;color:#1a1a1a}
  h1{font-size:1.4rem;margin-bottom:.25rem}
  .subtitle{color:#666;margin-bottom:2rem;font-size:.95rem}
  label{display:block;font-weight:500;margin-bottom:.3rem;margin-top:1rem}
  input{width:100%;box-sizing:border-box;border:1px solid #ccc;border-radius:6px;
        padding:.5rem .75rem;font-size:1rem}
  button{background:#7c3aed;color:#fff;border:none;border-radius:6px;
         padding:.55rem 1.4rem;font-size:1rem;cursor:pointer;margin-top:1.25rem}
  button:hover{background:#6d28d9}
  .hint{color:#666;font-size:.85rem;margin-top:.3rem}
  .security{background:#fef3c7;border:1px solid #fcd34d;border-radius:6px;
             padding:.75rem 1rem;font-size:.85rem;margin-bottom:1.5rem;color:#92400e}
  #mfa-row{display:none;margin-top:0}
  #result{display:none;margin-top:1.5rem}
  .token-box{font-family:monospace;font-size:.8rem;word-break:break-all;
             background:#f3f4f6;border:1px solid #d1d5db;border-radius:6px;
             padding:.75rem;margin:.5rem 0;user-select:all}
  .ok{background:#d1fae5;border:1px solid #6ee7b7;border-radius:6px;
      padding:.6rem 1rem;color:#065f46;font-size:.9rem;margin-bottom:.5rem}
  .err{background:#fee2e2;border:1px solid #fca5a5;border-radius:6px;
       padding:.6rem 1rem;color:#991b1b;font-size:.9rem}
  #spinner{display:none;color:#666;margin-top:.75rem;font-size:.9rem}
</style>
</head>
<body>
<h1>Get Obsidian Auth Token</h1>
<p class="subtitle">Sign in with your Obsidian account to generate a sync token for the add-on.</p>

<div class="security">
  🔒 Your credentials are sent directly to <strong>api.obsidian.md</strong> by this add-on server — they are not stored or logged anywhere.
</div>

<form id="form">
  <label for="email">Email address</label>
  <input id="email" name="email" type="email" autocomplete="email" required placeholder="you@example.com">

  <label for="password">Password</label>
  <input id="password" name="password" type="password" autocomplete="current-password" required>

  <div id="mfa-row">
    <label for="mfa">Two-factor code</label>
    <input id="mfa" name="mfa" type="text" inputmode="numeric" maxlength="6" placeholder="123456">
    <p class="hint">Enter the 6-digit code from your authenticator app.</p>
  </div>

  <button type="submit" id="btn">Get token</button>
  <p id="spinner">⏳ Signing in…</p>
</form>

<div id="result"></div>

<script>
const form = document.getElementById('form');
const result = document.getElementById('result');
const spinner = document.getElementById('spinner');
const btn = document.getElementById('btn');
const mfaRow = document.getElementById('mfa-row');

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  result.style.display = 'none';
  btn.disabled = true;
  spinner.style.display = 'block';

  const body = {
    email: document.getElementById('email').value,
    password: document.getElementById('password').value,
    mfa: document.getElementById('mfa').value || '',
  };

  try {
    const resp = await fetch('/signin', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    const data = await resp.json();
    result.style.display = 'block';

    if (data.needs_mfa) {
      mfaRow.style.display = 'block';
      result.innerHTML = '<div class="err">⚠️ Two-factor authentication required. Enter your 6-digit code above and try again.</div>';
      document.getElementById('mfa').focus();
    } else if (data.token) {
      result.innerHTML = ` + "`" + `<div class="ok">✓ Token generated successfully. Copy it into the add-on configuration as <strong>obsidian_auth_token</strong>.</div>
<p style="margin:.5rem 0 .2rem;font-weight:500">Your auth token:</p>
<div class="token-box" id="token-text">${data.token}</div>
<button type="button" onclick="navigator.clipboard.writeText(document.getElementById('token-text').textContent).then(()=>this.textContent='Copied!')" style="background:#059669;margin-top:.5rem">Copy to clipboard</button>` + "`" + `;
    } else {
      result.innerHTML = '<div class="err">❌ ' + (data.error || 'Sign-in failed. Check your email and password.') + '</div>';
    }
  } catch (err) {
    result.style.display = 'block';
    result.innerHTML = '<div class="err">❌ Request failed: ' + err.message + '</div>';
  } finally {
    btn.disabled = false;
    spinner.style.display = 'none';
  }
});
</script>
</body>
</html>`))

type signinRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	MFA      string `json:"mfa"`
}

type signinResponse struct {
	Token    string `json:"token,omitempty"`
	NeedsMFA bool   `json:"needs_mfa,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTmpl.Execute(w, nil)
}

func (s *Server) handleSignin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req signinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, signinResponse{Error: "invalid request body"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, signinResponse{Error: "email and password are required"})
		return
	}

	token, needsMFA, err := obsidianSignin(req.Email, req.Password, req.MFA)
	if err != nil {
		log.Printf("Obsidian signin error: %v", err)
		writeJSON(w, http.StatusOK, signinResponse{Error: err.Error()})
		return
	}
	if needsMFA {
		writeJSON(w, http.StatusOK, signinResponse{NeedsMFA: true})
		return
	}
	writeJSON(w, http.StatusOK, signinResponse{Token: token})
}

// obsidianSignin calls api.obsidian.md/user/signin and returns the token.
// Returns needsMFA=true when the server asks for a 2FA code.
func obsidianSignin(email, password, mfa string) (token string, needsMFA bool, err error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
		"mfa":      mfa,
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodPost, obsidianAPIBase+"/user/signin", bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://obsidian.md")

	// Preflight OPTIONS first, mirroring what obsidian-headless does.
	opts, _ := http.NewRequest(http.MethodOptions, obsidianAPIBase+"/user/signin", nil)
	opts.Header.Set("Origin", "https://obsidian.md")
	_, _ = client.Do(opts)

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("could not reach api.obsidian.md: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", false, fmt.Errorf("unexpected response from Obsidian API (HTTP %d)", resp.StatusCode)
	}

	if errMsg, ok := result["error"].(string); ok && errMsg != "" {
		if strings.Contains(errMsg, "2FA code") && !strings.Contains(errMsg, "incorrect") {
			return "", true, nil
		}
		return "", false, fmt.Errorf("%s", errMsg)
	}

	tok, _ := result["token"].(string)
	if tok == "" {
		return "", false, fmt.Errorf("no token in response (HTTP %d)", resp.StatusCode)
	}
	return tok, false, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
