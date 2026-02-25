package auth

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

// newTestManager creates a Manager with a temp config dir and a dummy
// credentials.json so that NewManager doesn't fail on missing creds.
func newTestManager(t *testing.T) *Manager {
	t.Helper()

	dir := t.TempDir()

	// Write a minimal valid credentials.json (installed app format).
	creds := `{
		"installed": {
			"client_id": "test-id.apps.googleusercontent.com",
			"client_secret": "test-secret",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://oauth2.googleapis.com/token",
			"redirect_uris": ["http://localhost"]
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestNewManager_DefaultPaths(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credsPath, []byte(`{"installed":{"client_id":"x","client_secret":"y","auth_uri":"https://a","token_uri":"https://t","redirect_uris":["http://localhost"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	if mgr.ConfigDir() != dir {
		t.Errorf("ConfigDir() = %q, want %q", mgr.ConfigDir(), dir)
	}
	if mgr.CredentialsFile() != credsPath {
		t.Errorf("CredentialsFile() = %q, want %q", mgr.CredentialsFile(), credsPath)
	}
}

func TestNewManager_CustomCredentialsPath(t *testing.T) {
	dir := t.TempDir()
	customCreds := filepath.Join(dir, "my-creds.json")
	if err := os.WriteFile(customCreds, []byte(`{"installed":{"client_id":"x","client_secret":"y","auth_uri":"https://a","token_uri":"https://t","redirect_uris":["http://localhost"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(dir, customCreds)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.CredentialsFile() != customCreds {
		t.Errorf("CredentialsFile() = %q, want %q", mgr.CredentialsFile(), customCreds)
	}
}

func TestNewManager_NoTokensFile(t *testing.T) {
	mgr := newTestManager(t)

	// Should have zero accounts when no tokens.json exists.
	accounts := mgr.ListAccounts()
	if len(accounts) != 0 {
		t.Errorf("ListAccounts() = %v, want empty", accounts)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	mgr := newTestManager(t)

	// Manually add an account with a token.
	mgr.config.Accounts["test"] = &Account{
		Email: "test@example.com",
		Token: &oauth2.Token{
			AccessToken:  "access-123",
			RefreshToken: "refresh-456",
			TokenType:    "Bearer",
		},
	}
	if err := mgr.save(); err != nil {
		t.Fatal(err)
	}

	// Verify tokens.json was created.
	tokensPath := mgr.tokensPath()
	if _, err := os.Stat(tokensPath); err != nil {
		t.Fatalf("tokens.json not created: %v", err)
	}

	// Load into a fresh manager and verify.
	mgr2, err := NewManager(mgr.configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	accounts := mgr2.ListAccounts()
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts() returned %d accounts, want 1", len(accounts))
	}
	if email, ok := accounts["test"]; !ok || email != "test@example.com" {
		t.Errorf("accounts[\"test\"] = %q, want \"test@example.com\"", email)
	}

	// Verify token fields survived the round-trip.
	mgr2.mu.RLock()
	acct := mgr2.config.Accounts["test"]
	mgr2.mu.RUnlock()
	if acct.Token.AccessToken != "access-123" {
		t.Errorf("AccessToken = %q, want \"access-123\"", acct.Token.AccessToken)
	}
	if acct.Token.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want \"refresh-456\"", acct.Token.RefreshToken)
	}
}

func TestListAccounts(t *testing.T) {
	mgr := newTestManager(t)

	mgr.config.Accounts["personal"] = &Account{Email: "me@gmail.com", Token: &oauth2.Token{}}
	mgr.config.Accounts["work"] = &Account{Email: "me@work.com", Token: &oauth2.Token{}}
	mgr.config.Accounts["nomail"] = &Account{Token: &oauth2.Token{}}

	accounts := mgr.ListAccounts()
	if len(accounts) != 3 {
		t.Fatalf("ListAccounts() returned %d accounts, want 3", len(accounts))
	}
	if accounts["personal"] != "me@gmail.com" {
		t.Errorf("personal = %q, want \"me@gmail.com\"", accounts["personal"])
	}
	if accounts["work"] != "me@work.com" {
		t.Errorf("work = %q, want \"me@work.com\"", accounts["work"])
	}
	if accounts["nomail"] != "" {
		t.Errorf("nomail = %q, want empty", accounts["nomail"])
	}
}

func TestResolveAccounts_Single(t *testing.T) {
	mgr := newTestManager(t)
	mgr.config.Accounts["personal"] = &Account{Token: &oauth2.Token{}}
	mgr.config.Accounts["work"] = &Account{Token: &oauth2.Token{}}

	names, err := mgr.ResolveAccounts("personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "personal" {
		t.Errorf("ResolveAccounts(\"personal\") = %v, want [\"personal\"]", names)
	}
}

func TestResolveAccounts_All(t *testing.T) {
	mgr := newTestManager(t)
	mgr.config.Accounts["personal"] = &Account{Token: &oauth2.Token{}}
	mgr.config.Accounts["work"] = &Account{Token: &oauth2.Token{}}

	names, err := mgr.ResolveAccounts("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("ResolveAccounts(\"all\") returned %d names, want 2", len(names))
	}

	// Check both names are present (order may vary with map iteration).
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["personal"] || !found["work"] {
		t.Errorf("ResolveAccounts(\"all\") = %v, want [\"personal\", \"work\"]", names)
	}
}

func TestResolveAccounts_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	mgr.config.Accounts["personal"] = &Account{Token: &oauth2.Token{}}

	_, err := mgr.ResolveAccounts("nonexistent")
	if err == nil {
		t.Error("ResolveAccounts(\"nonexistent\") returned nil error, want error")
	}
}

func TestResolveAccounts_AllEmpty(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ResolveAccounts("all")
	if err == nil {
		t.Error("ResolveAccounts(\"all\") with no accounts returned nil error, want error")
	}
}

func TestRemoveAccount(t *testing.T) {
	mgr := newTestManager(t)
	mgr.config.Accounts["personal"] = &Account{Email: "me@gmail.com", Token: &oauth2.Token{}}
	mgr.config.Accounts["work"] = &Account{Email: "me@work.com", Token: &oauth2.Token{}}
	if err := mgr.save(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RemoveAccount("personal"); err != nil {
		t.Fatal(err)
	}

	accounts := mgr.ListAccounts()
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts() after remove = %d, want 1", len(accounts))
	}
	if _, ok := accounts["personal"]; ok {
		t.Error("personal account still exists after removal")
	}

	// Verify persistence: reload and check.
	mgr2, err := NewManager(mgr.configDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(mgr2.ListAccounts()) != 1 {
		t.Error("removal was not persisted")
	}
}

func TestRemoveAccount_NotFound(t *testing.T) {
	mgr := newTestManager(t)

	err := mgr.RemoveAccount("nonexistent")
	if err == nil {
		t.Error("RemoveAccount(\"nonexistent\") returned nil error, want error")
	}
}

func TestOAuthConfig(t *testing.T) {
	mgr := newTestManager(t)

	cfg, err := mgr.oauthConfig([]string{"https://www.googleapis.com/auth/gmail.readonly"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "test-id.apps.googleusercontent.com" {
		t.Errorf("ClientID = %q, want \"test-id.apps.googleusercontent.com\"", cfg.ClientID)
	}
	if cfg.ClientSecret != "test-secret" {
		t.Errorf("ClientSecret = %q, want \"test-secret\"", cfg.ClientSecret)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Errorf("Scopes = %v, want [\"https://www.googleapis.com/auth/gmail.readonly\"]", cfg.Scopes)
	}
}

func TestOAuthConfig_MissingCredentials(t *testing.T) {
	dir := t.TempDir()
	// Don't create credentials.json — NewManager should still work (tokens may exist).
	// But oauthConfig should fail.
	mgr := &Manager{
		configDir:       dir,
		credentialsFile: filepath.Join(dir, "credentials.json"),
		config:          &Config{Accounts: make(map[string]*Account)},
	}

	_, err := mgr.oauthConfig([]string{"scope"})
	if err == nil {
		t.Error("oauthConfig with missing credentials returned nil error")
	}
}

func TestTokensFilePermissions(t *testing.T) {
	mgr := newTestManager(t)
	mgr.config.Accounts["test"] = &Account{Token: &oauth2.Token{AccessToken: "x"}}
	if err := mgr.save(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(mgr.tokensPath())
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("tokens.json permissions = %o, want 600", perm)
	}
}
