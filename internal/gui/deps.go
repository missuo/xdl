package gui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
)

// toolInfo is what the Dependencies tab shows of a tool, read off the main
// thread: finding a tool and asking its version run programs.
type toolInfo struct {
	path, version string
}

func (a *App) checkTools() {
	if a.toolsChecking {
		return
	}
	a.toolsChecking = true
	go func() {
		found := map[string]toolInfo{}
		for _, t := range backend.Tools {
			found[t.Name] = toolInfo{path: t.Path(), version: t.Version()}
		}
		a.win.Update(func() {
			a.tools, a.toolsChecked, a.toolsChecking = found, time.Now(), false
		})
	}()
}

func (a *App) depsSection(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Both are optional. XDL downloads no programs: it uses the copies your package manager installs.").
			FontSize(12).TextColor(t.TextMuted).Grow(1)
		label := "Check Again"
		if a.toolsChecking {
			label = "Checking…"
		}
		if ui.Button(c, label).Disabled(a.toolsChecking).Clicked() {
			a.checkTools()
		}
	})
	ui.Column(c).Gap(12).Children(func() {
		for _, tool := range backend.Tools {
			info, known := a.tools[tool.Name]
			card(c, "", func() {
				ui.Column(c).Gap(6).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, tool.Title).FontSize(14).Bold()
						switch {
						case !known:
							ui.Badge(c, "Checking…").Background(t.TextMuted.Alpha(0.12)).TextColor(t.TextMuted)
						case info.path == "":
							ui.Badge(c, "Not installed").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
						default:
							ui.Badge(c, "Installed").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
						}
					})
					ui.Text(c, tool.Purpose).FontSize(12).TextColor(t.TextMuted)
					if known && info.path != "" {
						text := info.path
						if info.version != "" {
							text = info.version + " · " + text
						}
						ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).Selectable()
					}
					if known && info.path == "" {
						cmd := tool.InstallCommand()
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, cmd).Font("monospace").FontSize(12).Selectable().Grow(1).
								Padding(6, 10).Radius(6).Background(t.Border.Alpha(0.35))
							if ui.Button(c, "Copy").Clicked() {
								c.WriteClipboard(cmd)
								a.toast("Copied — run it in a terminal, then Check Again")
							}
						})
					}
				})
			})
		}
	})
}
