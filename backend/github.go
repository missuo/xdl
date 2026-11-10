package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// verifyDownloadedDigest checks a downloaded file against a GitHub release
// asset's "sha256:<hex>" digest, when the release gives one.
func verifyDownloadedDigest(filePath, digest string) error {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return nil
	}
	expected := strings.TrimPrefix(strings.ToLower(digest), "sha256:")
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening the download: %v", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("verifying the download: %v", err)
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("the download's checksum does not match")
	}
	return nil
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}
