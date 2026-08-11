package backend

type DependencyVersionStatus struct {
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	// System means the binary comes from the OS package manager (Homebrew
	// etc.) rather than an app-managed download.
	System bool `json:"system,omitempty"`
}
