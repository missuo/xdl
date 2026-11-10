package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

// The X auth token lives in the system's keyring (the macOS Keychain, the
// Windows Credential Manager, the Secret Service on Linux), in a file only
// the user reads where there is no keyring, or in XDL_AUTH_TOKEN, which
// overrides both, for servers and containers. There are two slots, public
// and private, as the library has two kinds of fetches.

// AuthTokenEnv overrides the stored token.
const AuthTokenEnv = "XDL_AUTH_TOKEN"

const keyringService = "XDL"

// legacyKeyringService is where XDL 2.2 and earlier kept the token, a name
// they had from the app XDL began as; a token found there moves.
const legacyKeyringService = "TwitterXMediaBatchDownloader"

func tokenSlot(slot string) (string, error) {
	switch s := strings.ToLower(strings.TrimSpace(slot)); s {
	case "public", "private":
		return s, nil
	}
	return "", fmt.Errorf("no auth token slot %q (public or private)", slot)
}

func keyringAccount(slot string) string { return "auth-token-" + slot }

func tokenFile(slot string) string { return filepath.Join(Dir(), "auth_token_"+slot) }

// GetStoredAuthToken returns the token of a slot, "" for none.
func GetStoredAuthToken(slot string) (string, error) {
	slot, err := tokenSlot(slot)
	if err != nil {
		return "", err
	}
	if t := strings.TrimSpace(os.Getenv(AuthTokenEnv)); t != "" && slot == "public" {
		return t, nil
	}
	if t, err := keyring.Get(keyringService, keyringAccount(slot)); err == nil {
		return t, nil
	}
	if t, err := keyring.Get(legacyKeyringService, keyringAccount(slot)); err == nil && t != "" {
		if keyring.Set(keyringService, keyringAccount(slot), t) == nil {
			_ = keyring.Delete(legacyKeyringService, keyringAccount(slot))
		}
		return t, nil
	}
	if data, err := os.ReadFile(tokenFile(slot)); err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

// SetStoredAuthToken stores a slot's token; an empty one clears it.
func SetStoredAuthToken(slot, token string) error {
	slot, err := tokenSlot(slot)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ClearStoredAuthToken(slot)
	}
	_ = keyring.Delete(legacyKeyringService, keyringAccount(slot))
	if keyring.Set(keyringService, keyringAccount(slot), token) == nil {
		_ = os.Remove(tokenFile(slot))
		return nil
	}
	// No keyring: a file only the user can read.
	path := tokenFile(slot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

// ClearStoredAuthToken removes a slot's token.
func ClearStoredAuthToken(slot string) error {
	slot, err := tokenSlot(slot)
	if err != nil {
		return err
	}
	_ = os.Remove(tokenFile(slot))
	_ = keyring.Delete(legacyKeyringService, keyringAccount(slot))
	err = keyring.Delete(keyringService, keyringAccount(slot))
	if err == nil || errors.Is(err, keyring.ErrNotFound) || runtime.GOOS == "linux" {
		return nil // a Linux server may have no keyring to clear
	}
	return err
}

// AuthTokenStorage says where the token is kept.
func AuthTokenStorage() string {
	switch {
	case os.Getenv(AuthTokenEnv) != "":
		return "the " + AuthTokenEnv + " environment variable"
	case fileExists(tokenFile("public")):
		return tokenFile("public")
	case runtime.GOOS == "darwin":
		return "the macOS Keychain"
	case runtime.GOOS == "windows":
		return "the Windows Credential Manager"
	}
	return "the system keyring"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
