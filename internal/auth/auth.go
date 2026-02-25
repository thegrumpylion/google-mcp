// Package auth handles OAuth2 authentication and multi-account token management
// for Google APIs. It reads OAuth client credentials directly from a Google Cloud
// Console credentials.json file and stores per-account tokens separately.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// Config holds per-account tokens. OAuth client credentials are read
// directly from the Google credentials.json file, not stored here.
type Config struct {
	Accounts map[string]*Account `json:"accounts"`
}

// Account holds the OAuth2 token for a single Google account.
type Account struct {
	Email string        `json:"email,omitempty"`
	Token *oauth2.Token `json:"token"`
}

// Manager handles loading/saving tokens and reading credentials from the
// Google Cloud Console credentials.json file.
type Manager struct {
	mu              sync.RWMutex
	configDir       string
	credentialsFile string
	config          *Config
}

// NewManager creates a new auth manager.
//
// configDir defaults to $XDG_CONFIG_HOME/google-mcp (or ~/.config/google-mcp).
// credentialsFile defaults to <configDir>/credentials.json.
func NewManager(configDir, credentialsFile string) (*Manager, error) {
	if configDir == "" {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("could not determine home directory: %w", err)
			}
			xdg = filepath.Join(home, ".config")
		}
		configDir = filepath.Join(xdg, "google-mcp")
	}

	if credentialsFile == "" {
		credentialsFile = filepath.Join(configDir, "credentials.json")
	}

	m := &Manager{
		configDir:       configDir,
		credentialsFile: credentialsFile,
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// ConfigDir returns the configuration directory path.
func (m *Manager) ConfigDir() string {
	return m.configDir
}

// CredentialsFile returns the path to the Google credentials.json file.
func (m *Manager) CredentialsFile() string {
	return m.credentialsFile
}

func (m *Manager) tokensPath() string {
	return filepath.Join(m.configDir, "tokens.json")
}

func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.tokensPath())
	if errors.Is(err, os.ErrNotExist) {
		m.config = &Config{
			Accounts: make(map[string]*Account),
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading tokens: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing tokens: %w", err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = make(map[string]*Account)
	}
	m.config = &cfg
	return nil
}

func (m *Manager) save() error {
	if err := os.MkdirAll(m.configDir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tokens: %w", err)
	}
	if err := os.WriteFile(m.tokensPath(), data, 0o600); err != nil {
		return fmt.Errorf("writing tokens: %w", err)
	}
	return nil
}

// oauthConfig reads the credentials.json file and builds an oauth2.Config
// with the given scopes.
func (m *Manager) oauthConfig(scopes []string) (*oauth2.Config, error) {
	data, err := os.ReadFile(m.credentialsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("credentials file not found at %s\n\nDownload it from https://console.cloud.google.com/apis/credentials and place it there, or use --credentials to specify a different path", m.credentialsFile)
	}
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}

	cfg, err := google.ConfigFromJSON(data, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials file: %w", err)
	}
	return cfg, nil
}

// ListAccounts returns all configured account names and their email addresses.
func (m *Manager) ListAccounts() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	accounts := make(map[string]string, len(m.config.Accounts))
	for name, acct := range m.config.Accounts {
		accounts[name] = acct.Email
	}
	return accounts
}

// RemoveAccount removes an account by name.
func (m *Manager) RemoveAccount(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.config.Accounts[name]; !ok {
		return fmt.Errorf("account %q not found", name)
	}
	delete(m.config.Accounts, name)
	return m.save()
}

// Authenticate runs the OAuth2 authorization code flow for a named account.
// It opens a browser for consent, runs a local callback server, and stores
// the resulting token.
func (m *Manager) Authenticate(ctx context.Context, name string, scopes []string) error {
	cfg, err := m.oauthConfig(scopes)
	if err != nil {
		return err
	}

	// Start a local listener on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting local listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", port)

	// Channel to receive the auth code or error.
	type authResult struct {
		code string
		err  error
	}
	resultCh := make(chan authResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			resultCh <- authResult{err: fmt.Errorf("oauth error: %s", errMsg)}
			fmt.Fprintf(w, "Authorization failed: %s. You can close this tab.", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			resultCh <- authResult{err: fmt.Errorf("no authorization code received")}
			fmt.Fprint(w, "No authorization code received. You can close this tab.")
			return
		}
		resultCh <- authResult{code: code}
		fmt.Fprint(w, "Authorization successful! You can close this tab.")
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			resultCh <- authResult{err: fmt.Errorf("callback server error: %w", err)}
		}
	}()
	defer server.Shutdown(ctx)

	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("\nOpen this URL in your browser to authorize account %q:\n\n%s\n\nWaiting for authorization...\n", name, authURL)

	// Wait for the callback.
	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		token, err := cfg.Exchange(ctx, result.code)
		if err != nil {
			return fmt.Errorf("exchanging auth code for token: %w", err)
		}
		m.mu.Lock()
		m.config.Accounts[name] = &Account{Token: token}
		err = m.save()
		m.mu.Unlock()
		if err != nil {
			return err
		}
		fmt.Printf("Account %q authenticated successfully.\n", name)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TokenSource returns an oauth2.TokenSource for the named account.
// The token source automatically refreshes expired tokens and persists
// the updated token back to the tokens file.
func (m *Manager) TokenSource(ctx context.Context, name string, scopes []string) (oauth2.TokenSource, error) {
	m.mu.RLock()
	acct, ok := m.config.Accounts[name]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("account %q not found; run 'google-mcp auth add %s' first", name, name)
	}
	token := acct.Token
	m.mu.RUnlock()

	cfg, err := m.oauthConfig(scopes)
	if err != nil {
		return nil, err
	}

	ts := cfg.TokenSource(ctx, token)
	return &persistingTokenSource{
		base:    ts,
		manager: m,
		name:    name,
		orig:    token,
	}, nil
}

// ClientOption returns a google API option.ClientOption for the named account.
func (m *Manager) ClientOption(ctx context.Context, name string, scopes []string) (option.ClientOption, error) {
	ts, err := m.TokenSource(ctx, name, scopes)
	if err != nil {
		return nil, err
	}
	return option.WithTokenSource(ts), nil
}

// persistingTokenSource wraps a token source and saves refreshed tokens.
type persistingTokenSource struct {
	mu      sync.Mutex
	base    oauth2.TokenSource
	manager *Manager
	name    string
	orig    *oauth2.Token
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}

	// If the token was refreshed, persist it.
	if token.AccessToken != s.orig.AccessToken {
		s.orig = token
		s.manager.mu.Lock()
		if acct, ok := s.manager.config.Accounts[s.name]; ok {
			acct.Token = token
			_ = s.manager.save() // best-effort persist
		}
		s.manager.mu.Unlock()
	}
	return token, nil
}
