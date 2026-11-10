package gui

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
)

var libraryColumns = []ui.TableColumn{
	{Title: "Account", Sortable: true},
	{Title: "Name", Sortable: true},
	{Title: "Timeline", Width: 100, Sortable: true},
	{Title: "Items", Width: 80, Align: ui.End, Sortable: true},
	{Title: "Followers", Width: 90, Align: ui.End, Sortable: true},
	{Title: "Fetched", Width: 140, Sortable: true},
}

type libraryState struct {
	sort          ui.SortOrder
	confirmDelete bool
	deleteID      int64
	deleteName    string
}

func (a *App) filteredAccounts() []backend.AccountListItem {
	q := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.libFilter), "@"))
	out := make([]backend.AccountListItem, 0, len(a.accounts))
	for _, acc := range a.accounts {
		if q == "" || strings.Contains(strings.ToLower(acc.Username), q) || strings.Contains(strings.ToLower(acc.Name), q) {
			out = append(out, acc)
		}
	}
	s := a.lib.sort
	less := func(x, y backend.AccountListItem) int {
		switch s.Column {
		case "Account":
			return strings.Compare(strings.ToLower(x.Username), strings.ToLower(y.Username))
		case "Name":
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		case "Timeline":
			return strings.Compare(x.MediaType, y.MediaType)
		case "Items":
			return x.TotalMedia - y.TotalMedia
		case "Followers":
			return x.FollowersCount - y.FollowersCount
		case "Fetched":
			return strings.Compare(x.LastFetched, y.LastFetched)
		}
		return 0
	}
	if s.Column != "" {
		slices.SortStableFunc(out, func(x, y backend.AccountListItem) int {
			if s.Descending {
				return less(y, x)
			}
			return less(x, y)
		})
	}
	return out
}

func (a *App) libraryPage(c *ui.Context) {
	t := c.Theme()
	accounts := a.filteredAccounts()
	pageHeader(c, "Library", fmt.Sprintf("%d saved timelines. Downloading again skips what is already on disk.", len(a.accounts)), func() {
		ui.TextInput(c, &a.libFilter).Placeholder("Filter").Label("Filter accounts").Width(180)
		if ui.Button(c, "Import…").Tooltip("Import an exported .json timeline").Clicked() {
			a.importAccount()
		}
	})

	var sel *backend.AccountListItem
	if a.libRow >= 0 && a.libRow < len(accounts) {
		sel = &accounts[a.libRow]
	}
	a.libraryActions(c, sel)

	if len(accounts) == 0 {
		ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
			ui.Icon(c, iconLibrary).FontSize(36).TextColor(t.TextMuted.Alpha(0.5))
			msg := "Fetched accounts appear here."
			if a.libFilter != "" {
				msg = "No account matches the filter."
			}
			ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted)
		})
	} else {
		a.libList.Selected = &a.libRow
		a.libList.Sort = &a.lib.sort
		a.libList.Key = func(i int) any { return accounts[i].ID }
		table := ui.Table(c, &a.libList, libraryColumns, len(accounts), func(row, col int) {
			acc := accounts[row]
			switch col {
			case 0:
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "@"+acc.Username).FontSize(12.5).Bold().SingleLine()
					if !acc.Completed {
						ui.Badge(c, "partial").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
					}
				})
			case 1:
				ui.Text(c, acc.Name).FontSize(12.5).SingleLine()
			case 2:
				ui.Text(c, labelOf(acc.MediaType)).FontSize(12)
			case 3:
				ui.Text(c, fmt.Sprint(acc.TotalMedia)).FontSize(12)
			case 4:
				ui.Text(c, compact(acc.FollowersCount)).FontSize(12).TextColor(t.TextMuted)
			case 5:
				ui.Text(c, acc.LastFetched).FontSize(12).TextColor(t.TextMuted).SingleLine()
			}
		}).Grow(1).Basis(0)
		if table.Submitted() && sel != nil {
			a.libraryJob(*sel, engine.ModeDownload, false)
		}
		table.ContextMenu(func(m *ui.Menu) {
			if sel == nil {
				return
			}
			a.libraryMenu(c, m, *sel)
		})
	}

	switch ui.AlertDialog(c, &a.lib.confirmDelete, "Remove @"+a.lib.deleteName+" from the library?",
		"Downloaded files stay on disk.", "Cancel", "Remove") {
	case 1:
		if err := backend.DeleteAccount(a.lib.deleteID); err != nil {
			a.toast(err.Error())
		}
		a.libRow = -1
		go a.loadAccounts()
	}
}

func (a *App) libraryActions(c *ui.Context, sel *backend.AccountListItem) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Wrap().Children(func() {
		none := sel == nil
		if ui.PrimaryButton(c, "Update & Download").Disabled(none).
			Tooltip("Fetch what is new, then download everything missing").Clicked() {
			a.libraryJob(*sel, engine.ModeDownload, false)
		}
		if ui.Button(c, "Download Saved").Disabled(none).
			Tooltip("Download what the library holds without fetching").Clicked() {
			a.libraryJob(*sel, engine.ModeLibrary, false)
		}
		if ui.Button(c, "Full Refetch").Disabled(none).
			Tooltip("Fetch the whole timeline again and save it, without downloading").Clicked() {
			a.libraryJob(*sel, engine.ModeFetch, true)
		}
		if ui.Button(c, "Open Folder").Disabled(none).Clicked() {
			a.openPath(backend.GetFolderPath(a.downloadDir(), sel.Username))
		}
		ui.Spacer(c)
		ui.MenuButton(c, "More", func(m *ui.Menu) {
			if sel != nil {
				a.libraryMenu(c, m, *sel)
			}
		}).Disabled(none)
	})
}

func (a *App) libraryMenu(c *ui.Context, m *ui.Menu, acc backend.AccountListItem) {
	if m.Item("Update & Download").Chosen() {
		a.libraryJob(acc, engine.ModeDownload, false)
	}
	if m.Item("Download Saved").Chosen() {
		a.libraryJob(acc, engine.ModeLibrary, false)
	}
	if m.Item("Full Refetch").Chosen() {
		a.libraryJob(acc, engine.ModeFetch, true)
	}
	if m.Item("Redownload, Ignoring History").Tooltip("Download again what the history says was downloaded before; files still on disk are skipped").Chosen() {
		timeline := acc.MediaType
		if timeline == "" || timeline == "all" {
			timeline = "media"
		}
		a.submit(engine.Request{Username: acc.Username, TimelineType: timeline, Mode: engine.ModeLibrary, Redownload: true})
	}
	if m.Item("Subscribe (Check Daily)").Chosen() {
		timeline := acc.MediaType
		if timeline == "" || timeline == "all" {
			timeline = "media"
		}
		a.subscribe(acc.Username, timeline, 24*time.Hour)
	}
	m.Separator()
	if m.Item("Open Folder").Chosen() {
		a.openPath(backend.GetFolderPath(a.downloadDir(), acc.Username))
	}
	if acc.MediaType != "bookmarks" && m.Item("Open Profile on X").Chosen() {
		c.OpenURL("https://x.com/" + acc.Username)
	}
	gifs := backend.CheckGifsFolderHasMP4(a.downloadDir(), acc.Username)
	if m.Item("Convert GIFs to .gif").Disabled(!gifs || a.gifBusy || !backend.IsFFmpegInstalled()).
		Tooltip("Needs FFmpeg and a gifs folder of .mp4 files").Chosen() {
		a.convertGIFs(acc.Username)
	}
	if m.Item("Export Timeline…").Chosen() {
		a.exportAccount(acc)
	}
	m.Separator()
	if m.Item("Remove from Library…").Danger().Chosen() {
		a.lib.deleteID, a.lib.deleteName, a.lib.confirmDelete = acc.ID, acc.Username, true
	}
}

func (a *App) libraryJob(acc backend.AccountListItem, mode engine.Mode, full bool) {
	timeline := acc.MediaType
	if timeline == "" || timeline == "all" {
		timeline = "media"
	}
	a.submit(engine.Request{Username: acc.Username, TimelineType: timeline, Mode: mode, Full: full})
}

func (a *App) convertGIFs(username string) {
	a.gifBusy = true
	dir := backend.GetGifsFolderPath(a.downloadDir(), username)
	quality, resolution := a.cfg.GIFQuality, a.cfg.GIFResolution
	a.toast("Converting GIFs of @" + username + "…")
	go func() {
		converted, failed, err := backend.ConvertGIFsInFolder(dir, quality, resolution, false)
		msg := fmt.Sprintf("Converted %d GIFs of @%s (%d failed)", converted, username, failed)
		if err != nil {
			msg = "GIF conversion failed: " + err.Error()
		}
		log.Print(msg)
		a.win.Update(func() {
			a.gifBusy = false
			a.toast(msg)
		})
	}()
}

func (a *App) exportAccount(acc backend.AccountListItem) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "Export @" + acc.Username, ButtonLabel: "Export Here",
			Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path, err := backend.ExportAccountToFile(acc.ID, paths[0])
		a.win.Update(func() {
			if err != nil {
				a.toast("Export failed: " + err.Error())
				return
			}
			a.toast("Exported to " + path)
		})
	}()
}

func (a *App) importAccount() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "Import a Timeline",
			Filters: []mygo.FileFilter{{Name: "XDL export", Extensions: []string{"json"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		user, err := backend.ImportAccountFromFile(paths[0])
		a.win.Update(func() {
			if err != nil {
				a.toast("Import failed: " + err.Error())
				return
			}
			a.toast("Imported @" + user)
			go a.loadAccounts()
		})
	}()
}
