package ai

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Antigravity implements the Provider interface using Google OAuth2 login
// (same flow as Gemini CLI / Google Antigravity IDE).
// Instead of an API key, users authenticate via their Google account
// and the provider uses the OAuth access token as a Bearer token.
type Antigravity struct {
	model       string
	credentials *oauthCredentials
	mu          sync.Mutex
}

// oauthCredentials holds the cached OAuth2 tokens.
type oauthCredentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Email        string    `json:"email,omitempty"`
}

// These are injected at build time via:
//
//	go build -ldflags "-X 'github.com/DachengChen/paiSQL/ai.antigravityClientID=...' -X 'github.com/DachengChen/paiSQL/ai.antigravityClientSecret=...'"
//
// For local development, set ANTIGRAVITY_CLIENT_ID and ANTIGRAVITY_CLIENT_SECRET
// environment variables (e.g. in a .env file sourced before running).
var (
	antigravityClientID     string
	antigravityClientSecret string
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

// getAntigravityClientID returns the OAuth client ID, preferring the build-time
// value and falling back to the environment variable.
func getAntigravityClientID() string {
	if antigravityClientID != "" {
		return antigravityClientID
	}
	return os.Getenv("ANTIGRAVITY_CLIENT_ID")
}

// getAntigravityClientSecret returns the OAuth client secret, preferring the
// build-time value and falling back to the environment variable.
func getAntigravityClientSecret() string {
	if antigravityClientSecret != "" {
		return antigravityClientSecret
	}
	return os.Getenv("ANTIGRAVITY_CLIENT_SECRET")
}

var antigravityScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

var _ Provider = (*Antigravity)(nil)

// NewAntigravity creates an Antigravity provider.
// It loads cached credentials if available; otherwise the user must call Login().
func NewAntigravity(model string) *Antigravity {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	a := &Antigravity{model: model}
	// Try to load cached credentials
	if creds, err := loadAntigravityCredentials(); err == nil {
		a.credentials = creds
	}
	return a
}

func (a *Antigravity) Name() string {
	return fmt.Sprintf("Antigravity (%s)", a.model)
}

func (a *Antigravity) Chat(ctx context.Context, messages []Message) (string, error) {
	return a.call(ctx, systemPromptChat, messages)
}

func (a *Antigravity) SuggestIndexes(ctx context.Context, query string, explainJSON string) (string, error) {
	messages := []Message{
		{Role: "user", Content: fmt.Sprintf("Query:\n%s\n\nEXPLAIN output:\n%s", query, explainJSON)},
	}
	return a.call(ctx, systemPromptIndex, messages)
}

// IsLoggedIn returns true if valid credentials are cached.
func (a *Antigravity) IsLoggedIn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.credentials != nil && a.credentials.RefreshToken != ""
}

// LoggedInEmail returns the email of the logged-in user, or empty string.
func (a *Antigravity) LoggedInEmail() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.credentials != nil {
		return a.credentials.Email
	}
	return ""
}

// Login performs the OAuth2 authorization code flow:
// 1. Starts a local HTTP server on a random port
// 2. Opens the Google login page in the browser
// 3. Receives the authorization code via callback
// 4. Exchanges the code for access + refresh tokens
// 5. Caches the tokens to disk
//
// Returns the auth URL for display and a channel that signals completion.
func (a *Antigravity) Login() (authURL string, done <-chan error, err error) {
	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("failed to start OAuth callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)

	// Generate a random state parameter for CSRF protection
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		listener.Close()
		return "", nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Build the authorization URL
	params := url.Values{
		"client_id":     {getAntigravityClientID()},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(antigravityScopes, " ")},
		"access_type":   {"offline"},
		"state":         {state},
		"prompt":        {"consent"},
	}
	authURL = googleAuthURL + "?" + params.Encode()

	doneCh := make(chan error, 1)

	// Start the callback server
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}

	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			go func() {
				time.Sleep(500 * time.Millisecond)
				server.Close()
			}()
		}()

		q := r.URL.Query()

		// Check for errors
		if errCode := q.Get("error"); errCode != "" {
			errDesc := q.Get("error_description")
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h2>❌ Authentication failed</h2><p>%s: %s</p></body></html>", errCode, errDesc)
			doneCh <- fmt.Errorf("OAuth error: %s — %s", errCode, errDesc)
			return
		}

		// Verify state
		if q.Get("state") != state {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><body><h2>❌ State mismatch</h2><p>Possible CSRF attack.</p></body></html>")
			doneCh <- fmt.Errorf("OAuth state mismatch")
			return
		}

		code := q.Get("code")
		if code == "" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><body><h2>❌ No authorization code</h2></body></html>")
			doneCh <- fmt.Errorf("no authorization code received")
			return
		}

		// Exchange the code for tokens
		creds, err := exchangeCodeForTokens(code, redirectURI)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h2>❌ Token exchange failed</h2><p>%s</p></body></html>", err)
			doneCh <- err
			return
		}

		// Save credentials
		a.mu.Lock()
		a.credentials = creds
		a.mu.Unlock()

		// Fetch user email
		if email, err := fetchUserEmail(creds.AccessToken); err == nil {
			creds.Email = email
			a.mu.Lock()
			a.credentials.Email = email
			a.mu.Unlock()
		}

		if err := saveAntigravityCredentials(creds); err != nil {
			doneCh <- fmt.Errorf("failed to save credentials: %w", err)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<h2>✅ Authentication successful!</h2>
			<p>You can close this tab and return to paiSQL.</p>
			<script>setTimeout(function(){window.close()},3000);</script>
		</body></html>`)
		doneCh <- nil
	})

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			doneCh <- fmt.Errorf("OAuth server error: %w", err)
		}
	}()

	// Try to open the browser
	openBrowser(authURL)

	return authURL, doneCh, nil
}

// Logout clears cached credentials.
func (a *Antigravity) Logout() error {
	a.mu.Lock()
	a.credentials = nil
	a.mu.Unlock()
	return clearAntigravityCredentials()
}

// call makes a request to the Gemini API using OAuth Bearer token.
func (a *Antigravity) call(ctx context.Context, system string, messages []Message) (string, error) {
	token, err := a.getAccessToken()
	if err != nil {
		return "", err
	}

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{
			Role:  role,
			Parts: []part{{Text: m.Content}},
		})
	}

	body := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []part{{Text: system}},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	// Use Bearer token instead of API key
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", a.model)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("antigravity request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token might be expired; try refresh and retry once
		if refreshErr := a.refreshAccessToken(); refreshErr != nil {
			return "", fmt.Errorf("antigravity auth expired, re-login required: %w", refreshErr)
		}
		return a.call(ctx, system, messages)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("antigravity API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("antigravity parse error: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("antigravity returned no content")
	}

	var text string
	for _, p := range result.Candidates[0].Content.Parts {
		text += p.Text
	}

	return text, nil
}

// getAccessToken returns a valid access token, refreshing if expired.
func (a *Antigravity) getAccessToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.credentials == nil {
		return "", fmt.Errorf("not logged in to Antigravity. Use ':antigravity login' to authenticate")
	}

	// If token is still valid (with 1 minute buffer), use it
	if a.credentials.AccessToken != "" && time.Now().Before(a.credentials.ExpiresAt.Add(-1*time.Minute)) {
		return a.credentials.AccessToken, nil
	}

	// Need to refresh
	a.mu.Unlock()
	err := a.refreshAccessToken()
	a.mu.Lock()
	if err != nil {
		return "", err
	}

	return a.credentials.AccessToken, nil
}

// refreshAccessToken uses the refresh token to get a new access token.
func (a *Antigravity) refreshAccessToken() error {
	a.mu.Lock()
	refreshToken := a.credentials.RefreshToken
	a.mu.Unlock()

	if refreshToken == "" {
		return fmt.Errorf("no refresh token available, re-login required")
	}

	data := url.Values{
		"client_id":     {getAntigravityClientID()},
		"client_secret": {getAntigravityClientSecret()},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm(googleTokenURL, data)
	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh error (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("token refresh parse error: %w", err)
	}

	a.mu.Lock()
	a.credentials.AccessToken = tokenResp.AccessToken
	a.credentials.TokenType = tokenResp.TokenType
	a.credentials.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	a.mu.Unlock()

	return saveAntigravityCredentials(a.credentials)
}

// exchangeCodeForTokens exchanges an authorization code for tokens.
func exchangeCodeForTokens(code, redirectURI string) (*oauthCredentials, error) {
	data := url.Values{
		"client_id":     {getAntigravityClientID()},
		"client_secret": {getAntigravityClientSecret()},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.PostForm(googleTokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange error (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("token parse error: %w", err)
	}

	return &oauthCredentials{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// fetchUserEmail retrieves the user's email from Google's UserInfo endpoint.
func fetchUserEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo request failed: %d", resp.StatusCode)
	}

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}

// credentialsPath returns the path to the cached credentials file.
func credentialsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".paisql", "antigravity_credentials.json"), nil
}

// loadAntigravityCredentials loads cached credentials from disk.
func loadAntigravityCredentials() (*oauthCredentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var creds oauthCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// saveAntigravityCredentials saves credentials to disk with restrictive permissions.
func saveAntigravityCredentials(creds *oauthCredentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// clearAntigravityCredentials removes the cached credentials file.
func clearAntigravityCredentials() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start() //nolint:errcheck
}
