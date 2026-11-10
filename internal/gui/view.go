package gui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/internal/engine"
)

func (a *App) view(c *ui.Context) {
	for _, m := range toastQueue {
		c.Toast(m)
	}
	toastQueue = nil
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).Basis(0).Background(t.Background).Children(func() {
			// The window controls sit over the sidebar: the page starts
			// at the top, its header dragging the window.
			bar := c.TitleBar()
			body := ui.Column(c).Grow(1).Padding(max(bar.Height, 28)+6, 28, 22).Gap(16)
			body.Children(func() {
				switch a.page {
				case pageDownload:
					a.downloadPage(c)
				case pageQueue:
					a.queuePage(c)
				case pageLibrary:
					a.libraryPage(c)
				case pageProfile:
					a.profilePage(c)
				case pageSettings:
					a.settingsPage(c)
				case pageSubs:
					a.subscriptionsPage(c)
				case pageLogs:
					a.logsPage(c)
				}
			})
		})
	})
}

func (a *App) sidebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	queued, running := 0, 0
	for _, j := range a.jobs {
		switch j.State {
		case engine.StateQueued:
			queued++
		case engine.StateFetching, engine.StateDownloading:
			running++
		}
	}
	ui.Column(c).Width(212).Shrink(0).Background(t.Surface).Border(0, t.Border).Children(func() {
		// Under the window controls, level with the page's title.
		ui.Column(c).Padding(max(bar.Height, 28)+6, 20, 14).Gap(1).Shrink(0).DragWindow().Children(func() {
			ui.Text(c, "XDL").FontSize(22).Bold()
			ui.Text(c, "X media downloader").FontSize(11.5).TextColor(t.TextMuted)
		})
		page := a.page
		if ui.Sidebar(c, &page, func() {
			ui.SidebarSection(c, "Download", nil, func() {
				ui.SidebarItem(c, pageDownload, iconDownload, "New Download")
				ui.SidebarItem(c, pageQueue, iconQueue, "Queue").Children(func() {
					if n := queued + running; n > 0 {
						ui.Badge(c, fmt.Sprint(n))
					}
				})
				ui.SidebarItem(c, pageSubs, iconSchedule, "Subscriptions").Children(func() {
					if len(a.subs) > 0 {
						ui.Badge(c, fmt.Sprint(len(a.subs)))
					}
				})
				ui.SidebarItem(c, pageLibrary, iconLibrary, "Library").Children(func() {
					if len(a.accounts) > 0 {
						ui.Badge(c, fmt.Sprint(len(a.accounts)))
					}
				})
			})
			ui.SidebarSection(c, "App", nil, func() {
				ui.SidebarItem(c, pageSettings, iconSettings, "Settings")
				ui.SidebarItem(c, pageLogs, iconLogs, "Logs")
			})
		}).Grow(1).Label("Pages").Changed() {
			a.navigate(page)
		}
		a.sidebarStatus(c)
	})
}

func (a *App) navigate(page string) {
	a.page = page
	switch page {
	case pageSettings:
		a.cfg = a.store.Get()
		a.loadNumbers()
		go a.loadToken()
		go a.loadHistoryCount()
	case pageSubs:
		go a.loadSubscriptions()
	case pageLibrary:
		go a.loadAccounts()
	}
}

func (a *App) sidebarStatus(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(12, 20, 18).Gap(6).Children(func() {
		dot, label := t.Success, "Browser bridge on :"+port(a.srv.Addr())
		if a.apiErr != "" {
			dot, label = t.Danger, "Extension bridge unavailable"
		}
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Width(7).Height(7).Radius(4).Background(dot)
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}).Tooltip(a.apiErr)
		for _, j := range a.jobs {
			if j.State == engine.StateFetching || j.State == engine.StateDownloading {
				ui.Text(c, j.Label()).FontSize(11.5).Bold().SingleLine()
				if j.State == engine.StateDownloading {
					ui.Progress(c, float64(j.Percent)/100).Label("Download progress")
				} else {
					ui.Progress(c, -1).Label("Fetching")
				}
				ui.Text(c, j.Summary()).FontSize(11).TextColor(t.TextMuted).SingleLine()
				break
			}
		}
	})
}

// card is a titled panel.
func card(c *ui.Context, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16, 18).Gap(12).Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(13).Bold()
		}
		body()
	})
}

// pageHeader is a page's title row; actions go on its right.
func pageHeader(c *ui.Context, title, subtitle string, actions func()) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).DragWindow().Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(22).Bold()
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(12).TextColor(t.TextMuted)
			}
		})
		if actions != nil {
			actions()
		}
	})
}

func stateColor(t *ui.Theme, s engine.State) ui.Color {
	switch s {
	case engine.StateDone:
		return t.Success
	case engine.StateFailed:
		return t.Danger
	case engine.StateCancelled:
		return t.Warning
	case engine.StateQueued:
		return t.TextMuted
	}
	return t.Accent
}

var stateNames = map[engine.State]string{
	engine.StateQueued:      "Queued",
	engine.StateFetching:    "Fetching",
	engine.StateDownloading: "Downloading",
	engine.StateDone:        "Done",
	engine.StateFailed:      "Failed",
	engine.StateCancelled:   "Cancelled",
}

func stateBadge(c *ui.Context, s engine.State) {
	t := c.Theme()
	col := stateColor(t, s)
	ui.Badge(c, stateNames[s]).Background(col.Alpha(0.15)).TextColor(col)
}

var sourceNames = map[string]string{
	"gui": "App", "cli": "CLI", "extension": "Browser", "api": "API", "clipboard": "Clipboard",
	"schedule": "Subscription", "agent": "Agent",
}

func port(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

func compact(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fK", float64(n)/1e3)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}
