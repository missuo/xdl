package gui

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// The icons come from IconMCP (https://iconmcp.com, MIT licence): outline
// style on a 16-unit grid, stroked in the text's color.
//
//go:embed icons/*.svg
var iconFiles embed.FS

// appIcon is the app's icon for the update window.
//
//go:embed appicon.png
var appIcon []byte

func icon(name string) *ui.SVG {
	data, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

var (
	iconDownload  = icon("download")
	iconQueue     = icon("queue")
	iconLibrary   = icon("media-library")
	iconSettings  = icon("settings")
	iconLogs      = icon("logs")
	iconFolder    = icon("folder-open")
	iconStop      = icon("stop-circle")
	iconCheck     = icon("circle-check")
	iconBefore    = icon("check")
	iconX         = icon("x-circle")
	iconSkip      = icon("minus-circle")
	iconSchedule  = icon("cron-schedule")
	iconAgent     = icon("ai-agent")
	iconExtension = icon("puzzle-piece")
)
