package gui

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
)

var timelineLabels = []string{"Media", "Timeline", "Tweets", "With Replies", "Likes", "Bookmarks"}

func timelineOf(label string) string {
	for i, l := range timelineLabels {
		if l == label {
			return engine.TimelineTypes[i]
		}
	}
	return "media"
}

func labelOf(timeline string) string {
	for i, t := range engine.TimelineTypes {
		if t == timeline {
			return timelineLabels[i]
		}
	}
	return "Media"
}

type downloadForm struct {
	label string
	full  bool
}

func (a *App) downloadPage(c *ui.Context) {
	pageHeader(c, "New Download", "Fetch an account's media timeline and download everything in it.", nil)
	a.newDownloadCard(c)
	if j, ok := a.focused(); ok {
		a.jobCard(c, j)
	} else {
		t := c.Theme()
		ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
			ui.Icon(c, iconDownload).FontSize(36).TextColor(t.TextMuted.Alpha(0.5))
			ui.Text(c, "Nothing downloading").FontSize(14).Bold().TextColor(t.TextMuted)
			ui.Text(c, "Enter an account above, send one from the browser extension, or run `xdl download <user>`.").
				FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(420)
		})
	}
}

func (a *App) startFromForm(mode engine.Mode) {
	timeline := timelineOf(a.form.label)
	handle := engine.NormalizeHandle(a.handle)
	if handle == "" && timeline != "bookmarks" {
		a.toast("Enter a username or a profile URL")
		return
	}
	a.submit(engine.Request{Username: handle, TimelineType: timeline, Mode: mode, Full: a.form.full})
	a.handle = ""
}

func (a *App) newDownloadCard(c *ui.Context) {
	t := c.Theme()
	card(c, "", func() {
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Text(c, "Account").FontSize(11.5).TextColor(t.TextMuted)
				known := make([]string, 0, len(a.accounts))
				for _, acc := range a.accounts {
					if acc.Username != "bookmarks" {
						known = append(known, "@"+acc.Username)
					}
				}
				if ui.Autocomplete(c, &a.handle, known).Placeholder("@username or profile URL").
					Label("Account").Disabled(a.form.label == "Bookmarks").Submitted() {
					a.startFromForm(engine.ModeDownload)
				}
			})
			ui.Column(c).Width(150).Gap(5).Children(func() {
				ui.Text(c, "Timeline").FontSize(11.5).TextColor(t.TextMuted)
				ui.Select(c, &a.form.label, timelineLabels).Label("Timeline")
			})
		})
		ui.Row(c).Gap(14).AlignItems(ui.Center).Wrap().Children(func() {
			changed := false
			changed = ui.Checkbox(c, &a.cfg.IncludePhotos, "Photos").Changed() || changed
			changed = ui.Checkbox(c, &a.cfg.IncludeVideos, "Videos").Changed() || changed
			changed = ui.Checkbox(c, &a.cfg.IncludeGifs, "GIFs").Changed() || changed
			changed = ui.Checkbox(c, &a.cfg.IncludeText, "Text").Changed() || changed
			changed = ui.Checkbox(c, &a.cfg.Retweets, "Retweets").Changed() || changed
			if changed {
				a.saveSettings()
			}
			ui.Checkbox(c, &a.form.full, "Full refetch").Tooltip("Fetch the whole timeline again instead of stopping where the library's copy begins")
			ui.Spacer(c)
			if ui.Button(c, "Fetch Only").Tooltip("Save the timeline to the library without downloading").Clicked() {
				a.startFromForm(engine.ModeFetch)
			}
			if ui.PrimaryButton(c, "Download").Clicked() {
				a.startFromForm(engine.ModeDownload)
			}
		})
	}).Shrink(0)
}

func (a *App) jobCard(c *ui.Context, j engine.Job) {
	t := c.Theme()
	card(c, "", func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			name, nick, picture := j.Username, j.Label(), ""
			if j.Account != nil {
				if j.Account.Nick != "" {
					nick = j.Account.Nick
				}
				picture = j.Account.ProfileImage
			}
			ui.Avatar(c, nick, a.avatar(picture)).Width(44).Height(44)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, nick).FontSize(15).Bold().SingleLine()
					stateBadge(c, j.State)
				})
				var parts []string
				if j.TimelineType != "bookmarks" {
					parts = append(parts, "@"+name)
				}
				parts = append(parts, labelOf(j.TimelineType))
				if j.Account != nil && j.Account.FollowersCount > 0 {
					parts = append(parts, compact(j.Account.FollowersCount)+" followers")
				}
				if j.Fetched > 0 {
					parts = append(parts, fmt.Sprintf("%d items (%d new)", j.Fetched, j.New))
				}
				ui.Text(c, strings.Join(parts, " · ")).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			if !j.State.Finished() {
				if ui.Button(c, "Stop").Clicked() {
					a.eng.Cancel(j.ID)
				}
			} else if j.Mode != engine.ModeFetch {
				if ui.Button(c, "Download Again").Tooltip("Download what the library holds; existing files are skipped").Clicked() {
					a.submit(engine.Request{Username: j.Username, TimelineType: j.TimelineType, Mode: engine.ModeLibrary})
				}
			}
			if j.OutputDir != "" {
				if ui.Button(c, "Open Folder").Clicked() {
					a.openPath(a.jobFolder(j))
				}
			}
		})
		switch j.State {
		case engine.StateQueued:
			ui.Progress(c, 0).Label("Queued")
		case engine.StateFetching:
			ui.Progress(c, -1).Label("Fetching")
		case engine.StateDownloading:
			ui.Progress(c, float64(j.Percent)/100).Label("Downloading")
		}
		status := j.Summary()
		col := t.TextMuted
		if j.State == engine.StateFailed {
			col = t.Danger
		}
		if j.State == engine.StateDownloading || j.State.Finished() && j.Total > 0 {
			status += fmt.Sprintf("  ·  %d downloaded, %d already downloaded, %d failed", j.Downloaded, j.Skipped+j.Known, j.Failed)
		}
		ui.Text(c, status).FontSize(12).TextColor(col).Selectable()
	}).Shrink(0)

	if len(a.items) > 0 && a.itemsJob == j.ID {
		a.itemsTable(c)
	} else if j.State == engine.StateFetching {
		ui.Column(c).Grow(1)
	}
}

// jobFolder is where a job's files land: the account's folder when the
// folder template is the default one.
func (a *App) jobFolder(j engine.Job) string {
	if j.TimelineType != "bookmarks" && j.TimelineType != "likes" &&
		strings.TrimSpace(a.cfg.FolderTemplate) == "{handle}" {
		dir := backend.GetFolderPath(j.OutputDir, j.Username)
		if backend.CheckFolderExists(j.OutputDir, j.Username) {
			return dir
		}
	}
	return j.OutputDir
}

var itemColumns = []ui.TableColumn{
	{Title: "#", Width: 54, Align: ui.End, Fixed: true},
	{Title: "Type", Width: 72},
	{Title: "Date", Width: 150},
	{Title: "Tweet"},
	{Title: "Status", Width: 110},
}

func (a *App) itemsTable(c *ui.Context) {
	t := c.Theme()
	items := a.items
	a.itemsList.Selected = &a.itemRow
	a.itemsList.Key = func(i int) any { return i }
	table := ui.Table(c, &a.itemsList, itemColumns, len(items), func(row, col int) {
		it := items[row]
		switch col {
		case 0:
			ui.Textf(c, "%d", row+1).TextColor(t.TextMuted).FontSize(12)
		case 1:
			ui.Text(c, it.Type).FontSize(12)
		case 2:
			ui.Text(c, shortDate(it.Date)).FontSize(12).SingleLine()
		case 3:
			text := it.Content
			if text == "" {
				text = it.URL
			}
			ui.Text(c, strings.ReplaceAll(html.UnescapeString(text), "\n", " ")).FontSize(12).SingleLine()
		case 4:
			itemStatus(c, it.Status)
		}
	}).Grow(1).Basis(0).MinHeight(160)
	if table.Submitted() && a.itemRow >= 0 && a.itemRow < len(items) {
		a.openItem(items[a.itemRow])
	}
	table.ContextMenu(func(m *ui.Menu) {
		if a.itemRow < 0 || a.itemRow >= len(items) {
			return
		}
		it := items[a.itemRow]
		if m.Item("Open File").Disabled(it.Path == "").Chosen() {
			a.openPath(it.Path)
		}
		if m.Item("Show in Folder").Disabled(it.Path == "").Chosen() {
			a.openPath(filepath.Dir(it.Path))
		}
		if m.Item("Open Tweet on X").Chosen() {
			c.OpenURL(fmt.Sprintf("https://x.com/i/status/%d", int64(it.TweetID)))
		}
		if m.Item("Copy Media URL").Chosen() {
			c.WriteClipboard(it.URL)
		}
	})
}

func (a *App) openItem(it engine.Item) {
	if it.Path != "" {
		a.openPath(it.Path)
	}
}

func itemStatus(c *ui.Context, status string) {
	t := c.Theme()
	switch status {
	case "success":
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconCheck).TextColor(t.Success).FontSize(13)
			ui.Text(c, "Downloaded").FontSize(12)
		})
	case "skipped":
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSkip).TextColor(t.TextMuted).FontSize(13)
			ui.Text(c, "On disk").FontSize(12).TextColor(t.TextMuted)
		})
	case "failed":
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconX).TextColor(t.Danger).FontSize(13)
			ui.Text(c, "Failed").FontSize(12).TextColor(t.Danger)
		})
	case engine.StatusDownloadedBefore:
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconBefore).TextColor(t.TextMuted).FontSize(13)
			ui.Text(c, "Downloaded before").FontSize(12).TextColor(t.TextMuted)
		})
	case "cancelled":
		ui.Text(c, "Stopped").FontSize(12).TextColor(t.Warning)
	default:
		ui.Text(c, "—").FontSize(12).TextColor(t.TextMuted)
	}
}

// shortDate writes X's timestamps (Twitter's Ruby dates, or
// RFC 3339) as local "2026-01-02 15:04".
func shortDate(s string) string {
	for _, layout := range []string{time.RubyDate, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
	}
	return s
}
