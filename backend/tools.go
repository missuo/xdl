package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// FFmpeg (GIF conversion) and ExifTool (metadata in downloaded files) are
// optional, and XDL downloads neither: it uses the copies the system's
// package manager installed (Homebrew, winget, apt…), which it finds on the
// PATH or where those managers put them, since an app opened from Finder
// gets a short PATH.

// Tool is an external program XDL can use.
type Tool struct {
	Name    string // ffmpeg, exiftool
	Title   string
	Purpose string
	binary  string
	// versionArgs prints the version; versionPattern finds it.
	versionArgs    []string
	versionPattern *regexp.Regexp
}

var (
	FFmpeg = &Tool{
		Name: "ffmpeg", Title: "FFmpeg", Purpose: "Converts GIFs (which X serves as MP4) to .gif",
		binary: "ffmpeg", versionArgs: []string{"-hide_banner", "-version"},
		versionPattern: regexp.MustCompile(`ffmpeg version n?(\S+)`),
	}
	ExifTool = &Tool{
		Name: "exiftool", Title: "ExifTool", Purpose: "Writes the post's link into downloaded files' metadata",
		binary: "exiftool", versionArgs: []string{"-ver"},
		versionPattern: regexp.MustCompile(`^\s*([0-9][0-9.]*)`),
	}
	Tools = []*Tool{FFmpeg, ExifTool}
)

// InstallCommand is how to install the tool with the system's package
// manager.
func (t *Tool) InstallCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install " + t.binary
	case "windows":
		if t == FFmpeg {
			return "winget install Gyan.FFmpeg"
		}
		return "winget install OliverBetz.ExifTool"
	}
	if t == ExifTool {
		return "sudo apt install libimage-exiftool-perl  (or your distribution's exiftool package)"
	}
	return "sudo apt install ffmpeg  (or your distribution's ffmpeg package)"
}

// Path is where the tool is, "" when it is not installed.
func (t *Tool) Path() string {
	name := t.binary
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if p, err := exec.LookPath(t.binary); err == nil {
		return p
	}
	var dirs []string
	switch runtime.GOOS {
	case "darwin":
		dirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"}
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Microsoft", "WinGet", "Links"))
		}
		dirs = append(dirs, `C:\ProgramData\chocolatey\bin`)
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, "scoop", "shims"))
		}
	default:
		dirs = []string{"/usr/local/bin", "/usr/bin", "/snap/bin"}
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// Installed reports whether the tool is there.
func (t *Tool) Installed() bool { return t.Path() != "" }

var versions sync.Map // path → version

// Version is the installed tool's version, "" when it is not installed or
// does not say.
func (t *Tool) Version() string {
	path := t.Path()
	if path == "" {
		return ""
	}
	if v, ok := versions.Load(path); ok {
		return v.(string)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, _ := command(ctx, path, t.versionArgs...).Output()
	v := ""
	if m := t.versionPattern.FindSubmatch(out); m != nil {
		v = strings.TrimSuffix(string(m[1]), ",")
	}
	versions.Store(path, v)
	return v
}

// command runs a tool without a console window on Windows.
func command(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	hideConsole(cmd)
	return cmd
}

// Kept for the GUI and the engine.
func IsFFmpegInstalled() bool   { return FFmpeg.Installed() }
func IsExifToolInstalled() bool { return ExifTool.Installed() }

// gifSizes are the width and the frame rate of each GIF resolution.
var gifSizes = map[string]struct {
	width int
	fps   string
}{
	"high":   {800, "15"},
	"medium": {600, "10"},
	"low":    {400, "8"},
}

// ConvertMP4ToGIF makes a .gif of a GIF X served as MP4. Quality "fast"
// lets FFmpeg pick the colors; "better" makes a palette for the clip first.
// Resolution is original, high (800 px wide), medium (600) or low (400).
func ConvertMP4ToGIF(ctx context.Context, input, output, quality, resolution string) error {
	path := FFmpeg.Path()
	if path == "" {
		return errors.New("FFmpeg is not installed: " + FFmpeg.InstallCommand())
	}
	size, scaled := gifSizes[resolution]
	args := []string{"-hide_banner", "-loglevel", "error", "-i", input}
	if quality == "better" {
		scale := ""
		if scaled {
			scale = fmt.Sprintf("scale=%d:-1:flags=lanczos,", size.width)
		}
		args = append(args, "-lavfi", fmt.Sprintf("[0:v]%ssplit[a][b];[a]palettegen=stats_mode=full[p];[b][p]paletteuse=dither=sierra2_4a", scale))
		if scaled {
			args = append(args, "-r", size.fps)
		} else {
			args = append(args, "-r", "15")
		}
	} else if scaled {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-1", size.width))
	}
	args = append(args, "-loop", "0", "-y", output)
	out, err := command(ctx, path, args...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = os.Remove(output)
		return fmt.Errorf("FFmpeg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ConvertGIFsInFolder converts the MP4s of an account's GIFs folder to
// .gif, keeping the MP4s unless deleteOriginal.
func ConvertGIFsInFolder(dir, quality, resolution string, deleteOriginal bool) (converted, failed int, err error) {
	if !FFmpeg.Installed() {
		return 0, 0, errors.New("FFmpeg is not installed: " + FFmpeg.InstallCommand())
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, f := range files {
		if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".mp4") {
			continue
		}
		in := filepath.Join(dir, f.Name())
		out := strings.TrimSuffix(in, filepath.Ext(in)) + ".gif"
		if err := ConvertMP4ToGIF(context.Background(), in, out, quality, resolution); err != nil {
			failed++
			continue
		}
		converted++
		if deleteOriginal {
			_ = os.Remove(in)
		}
	}
	return converted, failed, nil
}

// writeComment puts the post's link and X's file name into a photo's or a
// video's Comment tag, when ExifTool is installed; other files are left as
// they are.
func writeComment(ctx context.Context, file, postURL, originalName string) error {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".jpg", ".jpeg", ".png", ".mp4":
	default:
		return nil
	}
	path := ExifTool.Path()
	if path == "" {
		return nil
	}
	comment := postURL
	if originalName != "" {
		comment += " | " + originalName
	}
	out, err := command(ctx, path, "-q", "-overwrite_original", "-Comment="+comment, file).CombinedOutput()
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("ExifTool: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
