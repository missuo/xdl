package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// lookupSystemBinary resolves a dependency installed outside the app —
// Homebrew, MacPorts, or anything on PATH. GUI apps launched from Finder get
// a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin), so the well-known install
// prefixes are probed explicitly.
func lookupSystemBinary(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join("/opt/homebrew/bin", name), // Homebrew (Apple Silicon)
			filepath.Join("/usr/local/bin", name),    // Homebrew (Intel)
			filepath.Join("/opt/local/bin", name),    // MacPorts
		}
	case "linux":
		candidates = []string{
			filepath.Join("/usr/local/bin", name),
			filepath.Join("/usr/bin", name),
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
