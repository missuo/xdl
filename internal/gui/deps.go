package gui

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/internal/deps"
)

// checkDeps asks the network for the tools' latest versions.
func (a *App) checkDeps() {
	if a.depChecking {
		return
	}
	a.depChecking = true
	go func() {
		results := make([]deps.Status, len(deps.All))
		var wg sync.WaitGroup
		for i, d := range deps.All {
			wg.Go(func() { results[i] = d.Status() })
		}
		wg.Wait()
		a.win.Update(func() {
			for i, d := range deps.All {
				a.depStatus[d.Name] = results[i]
			}
			a.depChecked, a.depChecking = true, false
		})
	}()
}

func (a *App) installDep(d deps.Dep) {
	if _, busy := a.depProgress[d.Name]; busy {
		return
	}
	a.depProgress[d.Name] = -1
	delete(a.depError, d.Name)
	go func() {
		var mu sync.Mutex
		var last time.Time
		err := d.Download(func(done, total int64) {
			mu.Lock()
			defer mu.Unlock()
			if time.Since(last) < 150*time.Millisecond && done != total {
				return
			}
			last = time.Now()
			p := -1.0
			if total > 0 {
				p = float64(done) / float64(total)
			}
			a.win.Update(func() { a.depProgress[d.Name] = p })
		})
		if err != nil {
			log.Printf("installing %s: %v", d.Name, err)
		} else {
			log.Printf("installed %s", d.Name)
		}
		status := d.Status()
		a.win.Update(func() {
			delete(a.depProgress, d.Name)
			a.depStatus[d.Name] = status
			if err != nil {
				a.depError[d.Name] = err.Error()
			} else {
				a.toast(d.Title + " installed")
			}
		})
	}()
}

func (a *App) depsSection(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Homebrew and other system installs are used when present; XDL downloads its own copies otherwise.").
			FontSize(12).TextColor(t.TextMuted).Grow(1)
		label := "Check for Updates"
		if a.depChecking {
			label = "Checking…"
		}
		if ui.Button(c, label).Disabled(a.depChecking).Clicked() {
			a.checkDeps()
		}
	})
	ui.Column(c).Gap(12).Children(func() {
		for _, d := range deps.All {
			st, known := a.depStatus[d.Name]
			installed := d.Installed()
			card(c, "", func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Gap(3).Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, d.Title).FontSize(14).Bold()
							switch {
							case !installed:
								col := t.Warning
								if d.Required {
									col = t.Danger
								}
								ui.Badge(c, "Not installed").Background(col.Alpha(0.15)).TextColor(col)
							case known && st.System:
								ui.Badge(c, "System").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
							case known && !st.UpToDate():
								ui.Badge(c, "Update available").Background(t.Accent.Alpha(0.15)).TextColor(t.Accent)
							default:
								ui.Badge(c, "Installed").Background(t.Success.Alpha(0.15)).TextColor(t.Success)
							}
						})
						ui.Text(c, d.Description).FontSize(12).TextColor(t.TextMuted)
						if known {
							var parts []string
							if st.InstalledVersion != "" {
								parts = append(parts, "installed "+st.InstalledVersion)
							}
							if st.LatestVersion != "" {
								parts = append(parts, "latest "+st.LatestVersion)
							}
							if len(parts) > 0 {
								ui.Text(c, strings.Join(parts, " · ")).FontSize(11.5).TextColor(t.TextMuted)
							}
						}
					})
					if p, busy := a.depProgress[d.Name]; busy {
						ui.Column(c).Width(160).Gap(4).Children(func() {
							ui.Progress(c, p).Label("Installing " + d.Title)
							if p >= 0 {
								ui.Text(c, fmt.Sprintf("%.0f%%", p*100)).FontSize(11).TextColor(t.TextMuted)
							}
						})
					} else if !(known && st.System) {
						label := "Install"
						if installed {
							label = "Reinstall"
							if known && !st.UpToDate() {
								label = "Update"
							}
						}
						var btn ui.Element
						if !installed && d.Required {
							btn = ui.PrimaryButton(c, label)
						} else {
							btn = ui.Button(c, label)
						}
						if btn.Clicked() {
							a.installDep(d)
						}
					}
				})
				if msg := a.depError[d.Name]; msg != "" {
					ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Selectable()
				}
			})
		}
	})
}

func (a *App) logsPage(c *ui.Context) {
	t := c.Theme()
	pageHeader(c, "Logs", "What XDL has been doing since it started.", func() {
		if ui.Button(c, "Copy").Disabled(len(a.logs) == 0).Clicked() {
			c.WriteClipboard(strings.Join(a.logs, "\n"))
			a.toast("Copied the log")
		}
	})
	lines := a.logs
	logList.FollowEnd = true
	ui.List(c, &logList, len(lines), func(i int) {
		ui.Text(c, lines[i]).Font("monospace").FontSize(11.5).Padding(2, 10).Selectable()
	}).Grow(1).Basis(0).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "Nothing yet.").TextColor(t.TextMuted).Padding(12)
		}
	})
}

var logList ui.ListState
