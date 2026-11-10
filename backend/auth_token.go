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

const authTokenKeyringService = "TwitterXMediaBatchDownloader"

func normalizeAuthTokenSlot(slot string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(slot)) {
	case "public":
		return "public", nil
	case "private":
		return "private", nil
	default:
		return "", fmt.Errorf("invalid auth token slot: %q", slot)
	}
}

func authTokenKeyringUser(slot string) string {
	return "auth-token-" + slot
}

// AuthTokenEnv overrides the stored token, for servers and containers.
const AuthTokenEnv = "XDL_AUTH_TOKEN"

// authTokenFile is where the token goes when the system has no keyring (a
// Linux server or a container without a Secret Service).
func authTokenFile(slot string) string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "XDL", "auth_token_"+slot)
}

func GetStoredAuthToken(slot string) (string, error) {
	normalizedSlot, err := normalizeAuthTokenSlot(slot)
	if err != nil {
		return "", err
	}
	if token := strings.TrimSpace(os.Getenv(AuthTokenEnv)); token != "" && normalizedSlot == "public" {
		return token, nil
	}

	token, err := keyring.Get(authTokenKeyringService, authTokenKeyringUser(normalizedSlot))
	if err == nil {
		return token, nil
	}
	// Not in the keyring, or no keyring at all: the file, if any.
	if data, ferr := os.ReadFile(authTokenFile(normalizedSlot)); ferr == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return "", nil
}

// AuthTokenStorage says where SetStoredAuthToken keeps the token.
func AuthTokenStorage() string {
	if os.Getenv(AuthTokenEnv) != "" {
		return "the " + AuthTokenEnv + " environment variable"
	}
	if _, err := os.Stat(authTokenFile("public")); err == nil {
		return authTokenFile("public")
	}
	switch runtime.GOOS {
	case "darwin":
		return "the macOS Keychain"
	case "windows":
		return "the Windows Credential Manager"
	}
	return "the system keyring"
}

func SetStoredAuthToken(slot, token string) error {
	normalizedSlot, err := normalizeAuthTokenSlot(slot)
	if err != nil {
		return err
	}

	if strings.TrimSpace(token) == "" {
		return ClearStoredAuthToken(normalizedSlot)
	}

	if err := keyring.Set(authTokenKeyringService, authTokenKeyringUser(normalizedSlot), token); err == nil {
		_ = os.Remove(authTokenFile(normalizedSlot))
		return nil
	}
	// No keyring: a file only the user can read.
	path := authTokenFile(normalizedSlot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

func ClearStoredAuthToken(slot string) error {
	normalizedSlot, err := normalizeAuthTokenSlot(slot)
	if err != nil {
		return err
	}
	_ = os.Remove(authTokenFile(normalizedSlot))

	err = keyring.Delete(authTokenKeyringService, authTokenKeyringUser(normalizedSlot))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil && runtime.GOOS == "linux" {
		return nil // no keyring to clear
	}
	return err
}
