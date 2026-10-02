package main

import (
	"context"
	"net/http"
	"strings"
	"unicode"

	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
)

type oauthStartRequest struct {
	Provider     string `json:"provider"`
	StorageName  string `json:"storageName"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

func safeRemoteName(name string) string {
	var b strings.Builder
	b.WriteString("abm-")
	for _, ch := range strings.ToLower(name) {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else if b.Len() > 4 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// handleOAuthStart owns the rclone authorization process. The browser may
// open automatically on a desktop; the temporary authorization URL and
// progress are also returned via /api/runs so the user never enters an
// interactive rclone configuration session or sees the resulting token.
func (g *guiServer) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	var req oauthStartRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, ok := provider.Get(req.Provider)
	if !ok || p.Backend != provider.BackendRclone || p.Auth != provider.AuthOAuth {
		writeErr(w, http.StatusBadRequest, "this provider does not use the supported OAuth connection flow")
		return
	}
	if req.StorageName == "" {
		writeErr(w, http.StatusBadRequest, "storageName is required")
		return
	}
	if p.RequiresOwnOAuthApp && (req.ClientID == "" || req.ClientSecret == "") {
		writeErr(w, http.StatusBadRequest, "OAuth Client ID and Client Secret are required")
		return
	}
	remote := safeRemoteName(req.StorageName)
	runID, rs := g.newRun("oauth", req.Provider)
	rs.Stage = "Waiting for authorization in your browser"

	go func() {
		token, err := rcloneRunner().AuthorizeOAuthProgress(context.Background(), p.RcloneBackend, req.ClientID, req.ClientSecret, func(url string) {
			g.mu.Lock()
			rs.Result = map[string]string{"authorizationUrl": url, "remote": remote}
			g.mu.Unlock()
		})
		if err == nil && token == "" {
			err = errEmptyOAuthToken
		}
		if err == nil {
			err = rcloneRunner().CreateOAuthRemote(context.Background(), remote, p.RcloneBackend, req.ClientID, req.ClientSecret, token)
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		rs.Done = true
		if err != nil {
			rs.Error = "authorization failed: " + err.Error()
			rs.Stage = "Failed"
			return
		}
		rs.Stage = "Connected"
		rs.Result = map[string]string{"remote": remote}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID, "remote": remote})
}

type oauthTokenError string

func (e oauthTokenError) Error() string { return string(e) }

const errEmptyOAuthToken oauthTokenError = "the provider returned no authorization token"
