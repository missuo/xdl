package backend

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// XDL 2.0.0 and earlier downloaded xtractor next to the library, and
// xtractor uploads auth tokens to its author. removeXtractor deletes what
// those versions left there, once per process.
var removeXtractorOnce sync.Once

func removeXtractor() {
	removeXtractorOnce.Do(func() {
		dir := filepath.Dir(GetDBPath())
		for _, name := range []string{
			"xtractor", "xtractor.exe", "extractor", "extractor.exe",
			"extractor.sha256", "xtractor-version.json", "xtractor-signer.json",
		} {
			path := filepath.Join(dir, name)
			err := os.Remove(path)
			switch {
			case err == nil:
				log.Printf("removed %s, which XDL 2.0.0 and earlier installed: it uploads auth tokens to its author — revoke the token you used with it", path)
			case !errors.Is(err, fs.ErrNotExist):
				log.Printf("could not remove %s: %v — delete it yourself", path, err)
			}
		}
	})
}
