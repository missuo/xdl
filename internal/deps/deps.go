// Package deps describes the external tools XDL uses: xtractor, which
// extracts timelines, FFmpeg, which converts GIFs, and ExifTool, which embeds
// metadata.
package deps

import (
	"fmt"
	"strings"

	"github.com/missuo/xdl/backend"
)

// Dep is one external tool.
type Dep struct {
	Name        string // xtractor, ffmpeg, exiftool
	Title       string
	Description string
	Required    bool
	Installed   func() bool
	Download    func(progress func(downloaded, total int64)) error
	status      func() Status
}

// Status is a tool's installed and latest versions. Status checks the
// network for the latest version.
type Status struct {
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	System           bool   `json:"system,omitempty"`
}

// Status fetches the tool's versions (it asks the network for the latest).
func (d Dep) Status() Status { return d.status() }

// UpToDate reports whether the installed version is the latest known.
func (s Status) UpToDate() bool {
	return s.Installed && (s.LatestVersion == "" || s.System ||
		strings.TrimPrefix(s.InstalledVersion, "v") == strings.TrimPrefix(s.LatestVersion, "v"))
}

// All lists the tools, required first.
var All = []Dep{
	{
		Name: "xtractor", Title: "xtractor", Required: true,
		Description: "Extracts timelines from X (required)",
		Installed:   backend.IsExtractorInstalled,
		Download:    backend.DownloadExtractor,
		status: func() Status {
			s := backend.GetExtractorVersionStatus()
			return Status{Installed: s.Installed, InstalledVersion: s.InstalledVersion, LatestVersion: s.LatestVersion}
		},
	},
	{
		Name: "ffmpeg", Title: "FFmpeg",
		Description: "Converts GIF videos to .gif",
		Installed:   backend.IsFFmpegInstalled,
		Download:    backend.DownloadFFmpeg,
		status: func() Status {
			s := backend.GetFFmpegVersionStatus()
			return Status(s)
		},
	},
	{
		Name: "exiftool", Title: "ExifTool",
		Description: "Embeds tweet metadata into files",
		Installed:   backend.IsExifToolInstalled,
		Download:    backend.DownloadExifTool,
		status: func() Status {
			s := backend.GetExifToolVersionStatus()
			return Status(s)
		},
	},
}

// Find returns the tool by name.
func Find(name string) (Dep, error) {
	for _, d := range All {
		if strings.EqualFold(d.Name, name) {
			return d, nil
		}
	}
	return Dep{}, fmt.Errorf("unknown dependency %q (xtractor, ffmpeg or exiftool)", name)
}
