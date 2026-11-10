package backend

import (
	"os"
	"path/filepath"
	"strings"
)

// Where an account's files go: <download folder>/<handle>/, with a folder
// for each kind of media (see download.go).

// GetDefaultDownloadPath is the download folder when the settings name
// none: the user's Pictures folder.
func GetDefaultDownloadPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "XDL")
	}
	return filepath.Join(home, "Pictures")
}

// GetFolderPath is an account's folder.
func GetFolderPath(downloads, handle string) string {
	return filepath.Join(downloads, handle)
}

// GetGifsFolderPath is the folder of an account's GIFs.
func GetGifsFolderPath(downloads, handle string) string {
	return filepath.Join(downloads, handle, kindFolder("animated_gif"))
}

// CheckFolderExists reports whether an account has a folder.
func CheckFolderExists(downloads, handle string) bool {
	info, err := os.Stat(GetFolderPath(downloads, handle))
	return err == nil && info.IsDir()
}

// CheckGifsFolderHasMP4 reports whether an account's GIFs folder holds a
// GIF as XDL downloads it (an MP4) to convert to .gif.
func CheckGifsFolderHasMP4(downloads, handle string) bool {
	files, err := os.ReadDir(GetGifsFolderPath(downloads, handle))
	if err != nil {
		return false
	}
	for _, f := range files {
		if !f.IsDir() && strings.EqualFold(filepath.Ext(f.Name()), ".mp4") {
			return true
		}
	}
	return false
}
