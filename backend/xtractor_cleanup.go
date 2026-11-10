package backend

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// XDL 2.0.0 and earlier downloaded binaries built by afkarxyz into the
// library's old folder: xtractor, which uploads auth tokens to its author,
// and FFmpeg. removeXtractor deletes them when the library opens.
func removeXtractor() {
	dir := legacyDir()
	for _, name := range []string{
		"xtractor", "xtractor.exe", "extractor", "extractor.exe",
		"extractor.sha256", "xtractor-version.json", "xtractor-signer.json",
		"ffmpeg", "ffmpeg.exe", "ffprobe", "ffprobe.exe", "ffmpeg-version.json",
	} {
		path := filepath.Join(dir, name)
		err := os.Remove(path)
		switch {
		case err == nil:
			log.Printf("removed %s, which XDL 2.0.0 and earlier downloaded from afkarxyz (see the security notice)", path)
		case !errors.Is(err, fs.ErrNotExist):
			log.Printf("could not remove %s: %v — delete it yourself", path, err)
		}
	}
}
