package gui

import (
	"fmt"
	"html"
	"io/fs"
	"log"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
)

// The profile page shows an account the library holds: its profile, and
// its media as a grid of thumbnails to filter, look through in a viewer and
// download one by one or all at once.

const pageProfile = "profile"

var (
	iconImage    = icon("image")
	iconVideo    = icon("video")
	iconGIF      = icon("gif")
	iconText     = icon("file-text")
	iconEye      = icon("eye")
	iconPrev     = icon("chevron-left")
	iconNext     = icon("chevron-right")
	iconClose    = icon("close")
	iconOpen     = icon("external-link")
	iconPin      = icon("pin")
	iconCalendar = icon("calendar")
	iconLink     = icon("link")
	iconVerified = icon("badge-check")
	iconLock     = icon("lock")
	iconPlay     = icon("play")
	iconBack     = icon("arrow-left")
)

type mediaFilter struct {
	label string
	types []string
	icon  *ui.SVG
}

var mediaFilters = []mediaFilter{
	{"All", nil, nil},
	{"Images", []string{"photo"}, iconImage},
	{"Videos", []string{"video"}, iconVideo},
	{"GIFs", []string{"animated_gif", "gif"}, iconGIF},
	{"Text", []string{"text"}, iconText},
}

var sortOrders = []string{"Newest first", "Oldest first"}

type profileState struct {
	username, timeline string

	loading   bool
	err       string
	info      backend.AccountInfo
	entries   []backend.TimelineEntry
	completed bool
	// saved holds the files downloaded, by media key: those the download
	// history has, and those of older downloads found in the account's
	// folder by their post's ID.
	saved map[string]string

	filter int
	order  string
	list   bool

	// view is the entries the filter keeps, in order, as indices into
	// entries; viewOf is what it was made for.
	view   []int
	viewOf string

	grid      ui.GridState
	gridRow   int
	selection ui.Selection[string]
	rows      ui.ListState
	row       int
	gridWidth float32 // the grid's width in the last frame, for square cells

	viewer   int // the position in view the viewer shows, -1 when closed
	video    *mygo.WebView
	videoURL string
}

// openProfile shows an account of the library.
func (a *App) openProfile(username, timeline string) {
	if timeline == "" || timeline == "all" {
		timeline = "media"
	}
	p := &a.prof
	if p.username != username || p.timeline != timeline {
		*p = profileState{username: username, timeline: timeline, order: sortOrders[0], viewer: -1, gridRow: -1, row: -1, video: p.video}
	}
	p.loading = true
	a.page = pageProfile
	folder := backend.GetFolderPath(a.downloadDir(), username)
	go a.loadProfile(username, timeline, folder)
}

func (a *App) loadProfile(username, timeline, folder string) {
	entries, info, completed, err := engine.SavedProfile(username, timeline)
	var saved map[string]string
	if err == nil {
		saved = downloadedFiles(entries, folder)
	}
	a.win.Update(func() {
		p := &a.prof
		if p.username != username || p.timeline != timeline {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.err, p.info, p.entries, p.completed, p.saved, p.viewOf = "", info, entries, completed, saved, ""
	})
}

var reTweetID = regexp.MustCompile(`\d{15,20}`)

// downloadedFiles finds the files of entries: in the download history, or
// else, for what was downloaded before XDL kept one, in folder by the
// post's ID in their names (the n-th file of a post for its n-th item).
func downloadedFiles(entries []backend.TimelineEntry, folder string) map[string]string {
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = backend.MediaKey(e.URL, int64(e.TweetID), e.Type)
	}
	saved, err := backend.DownloadedPaths(keys)
	if err != nil {
		log.Printf("profile: download history: %v", err)
	}
	if saved == nil {
		saved = map[string]string{}
	}
	if len(saved) == len(entries) {
		return saved
	}

	byPost := map[int64][]string{}
	root := strings.Count(folder, string(filepath.Separator))
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(path, string(filepath.Separator))-root > 3 {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.Contains(name, ".part") || strings.HasPrefix(name, ".") {
			return nil
		}
		for _, m := range reTweetID.FindAllString(name, -1) {
			if id, err := strconv.ParseInt(m, 10, 64); err == nil {
				byPost[id] = append(byPost[id], path)
			}
		}
		return nil
	})
	for id := range byPost {
		slices.Sort(byPost[id])
	}
	nth := map[int64]int{}
	for i, e := range entries {
		id := int64(e.TweetID)
		n := nth[id]
		nth[id]++
		if _, ok := saved[keys[i]]; ok {
			continue
		}
		if files := byPost[id]; n < len(files) {
			saved[keys[i]] = files[n]
		}
	}
	return saved
}

func (p *profileState) key(e backend.TimelineEntry) string {
	return backend.MediaKey(e.URL, int64(e.TweetID), e.Type)
}

// savedPath is the file downloaded for e, "" for none.
func (p *profileState) savedPath(e backend.TimelineEntry) string {
	return p.saved[p.key(e)]
}

func (p *profileState) count(f mediaFilter) int {
	if f.types == nil {
		return len(p.entries)
	}
	n := 0
	for _, e := range p.entries {
		if slices.Contains(f.types, e.Type) {
			n++
		}
	}
	return n
}

// refreshView remakes view when the filter, the order or the entries changed.
func (p *profileState) refreshView() {
	of := fmt.Sprintf("%d|%s|%d", p.filter, p.order, len(p.entries))
	if of == p.viewOf {
		return
	}
	p.viewOf = of
	f := mediaFilters[p.filter]
	p.view = p.view[:0]
	for i, e := range p.entries {
		if f.types == nil || slices.Contains(f.types, e.Type) {
			p.view = append(p.view, i)
		}
	}
	// The library keeps the timeline newest first.
	if p.order == sortOrders[1] {
		slices.Reverse(p.view)
	}
	if p.viewer >= len(p.view) {
		p.viewer = -1
	}
}

func (a *App) profilePage(c *ui.Context) {
	t := c.Theme()
	p := &a.prof
	ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).DragWindow().Children(func() {
		if iconButton(c, ui.Button(c, ""), iconBack, "Library").Tooltip("Back to the library").Clicked() {
			a.navigate(pageLibrary)
		}
		ui.Spacer(c)
		if p.loading {
			ui.Spinner(c)
		}
	})
	if p.err != "" {
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "This account could not be opened").Bold()
			ui.Text(c, p.err).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	if p.entries == nil && p.loading {
		return
	}
	p.refreshView()
	a.profileHeader(c)
	a.profileToolbar(c)
	if len(p.view) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "Nothing in this filter.").FontSize(13).TextColor(t.TextMuted)
		})
	} else if p.list {
		a.profileList(c)
	} else {
		a.profileGrid(c)
	}
	a.mediaViewer(c)
}

func (a *App) profileHeader(c *ui.Context) {
	t := c.Theme()
	p := &a.prof
	info := p.info
	banner := variant(info.BannerVariants, "1500x500", info.Banner)
	ui.Column(c).Shrink(0).Radius(12).Background(t.Surface).Border(1, t.Border).Clip().Children(func() {
		if banner != "" {
			ui.Box(c).Height(92).Shrink(0).Background(t.Border.Alpha(0.4)).Children(func() {
				if bm := a.images.get(banner, "", imgBanner); bm != nil {
					ui.Image(c, bm).Fill().Fit(ui.Cover)
				}
				ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Gradient(t.Surface.Alpha(0), t.Surface.Alpha(0.85), 180)
			})
		}
		ui.Row(c).Padding(0, 18, 14).Gap(14).AlignItems(ui.Start).Children(func() {
			top := float32(14)
			if banner != "" {
				top = -34
			}
			avatar := variant(info.AvatarVariants, "400x400", info.ProfileImage)
			ui.Box(c).Size(88, 88).Shrink(0).Margin(top, 0, 0, 0).Radius(44).Background(t.Surface).Center().Children(func() {
				a.roundPicture(c, avatar, firstOf(info.Nick, p.username), 80)
			})
			ui.Column(c).Grow(1).Basis(0).Gap(5).Padding(10, 0, 0, 0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, firstOf(info.Nick, p.username)).FontSize(18).Bold().SingleLine()
					if info.Verified || info.BlueVerified {
						ui.Icon(c, iconVerified).FontSize(16).TextColor(t.Accent)
					}
					if info.Protected {
						ui.Icon(c, iconLock).FontSize(15).TextColor(t.TextMuted).Tooltip("Protected posts")
					}
				})
				handle := "@" + p.username
				if p.timeline != "media" {
					handle += " · " + labelOf(p.timeline)
				}
				ui.Text(c, handle).FontSize(12.5).TextColor(t.TextMuted)
				if bio := strings.TrimSpace(html.UnescapeString(info.Bio)); bio != "" {
					ui.Text(c, bio).FontSize(12.5).MaxLines(3).Selectable()
				}
				ui.Row(c).Gap(14).AlignItems(ui.Center).Wrap().Children(func() {
					meta := func(ic *ui.SVG, text string) {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, ic).FontSize(13).TextColor(t.TextMuted)
							ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
						})
					}
					if info.Location != "" {
						meta(iconPin, info.Location)
					}
					if info.URL != "" {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Icon(c, iconLink).FontSize(13).TextColor(t.TextMuted)
							ui.Link(c, displayURL(info.URL), info.URL).FontSize(11.5)
						})
					}
					if joined := joinedDate(info.Date); joined != "" {
						meta(iconCalendar, "Joined "+joined)
					}
				})
				ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
					stat := func(n int, label string) {
						ui.Row(c).Gap(3).Children(func() {
							ui.Text(c, compact(n)).FontSize(12).Bold()
							ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
						})
					}
					stat(info.StatusesCount, "posts")
					stat(info.FriendsCount, "following")
					stat(info.FollowersCount, "followers")
					stat(info.MediaCount, "media")
				})
			})
		})
	})
}

func (a *App) profileToolbar(c *ui.Context) {
	t := c.Theme()
	p := &a.prof
	saved := 0
	for _, e := range p.entries {
		if p.savedPath(e) != "" {
			saved++
		}
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Wrap().Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			ui.Textf(c, "%d items", len(p.entries)).FontSize(14).Bold()
			status := fmt.Sprintf("%d downloaded", saved)
			if !p.completed {
				status += " · partial fetch"
			}
			ui.Text(c, status).FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		acc := backend.AccountListItem{Username: p.username, MediaType: p.timeline}
		if n := p.selection.Len(); n > 0 {
			if iconButton(c, ui.Button(c, ""), iconDownload, fmt.Sprintf("Download %d Selected", n)).Clicked() {
				var urls []string
				for k := range p.selection.All() {
					urls = append(urls, strings.SplitN(k, "|", 2)[0])
				}
				a.downloadOnly(urls)
				p.selection.Clear()
			}
			if ui.Button(c, "Clear Selection").Clicked() {
				p.selection.Clear()
			}
		}
		if iconButton(c, ui.PrimaryButton(c, ""), iconDownload, "Download All").
			Tooltip("Download what the library holds and is not downloaded yet").Clicked() {
			a.libraryJob(acc, engine.ModeLibrary, false)
		}
		if ui.Button(c, "Update").Tooltip("Fetch what is new, then download it").Clicked() {
			a.libraryJob(acc, engine.ModeDownload, false)
		}
		if iconButton(c, ui.Button(c, ""), iconFolder, "Open Folder").Clicked() {
			a.openPath(backend.GetFolderPath(a.downloadDir(), p.username))
		}
		ui.MenuButton(c, "More", func(m *ui.Menu) {
			if m.Item("Full Refetch").Tooltip("Fetch the whole timeline again, without downloading").Chosen() {
				a.libraryJob(acc, engine.ModeFetch, true)
			}
			if m.Item("Subscribe (Check Daily)").Chosen() {
				a.subscribe(p.username, p.timeline, 24*time.Hour)
			}
			if m.Item("Export Timeline…").Chosen() {
				if id, ok := a.libraryID(p.username, p.timeline); ok {
					acc.ID = id
					a.exportAccount(acc)
				}
			}
			if p.timeline != "bookmarks" && m.Item("Open Profile on X").Chosen() {
				c.OpenURL("https://x.com/" + p.username)
			}
		})
	})
	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Wrap().Children(func() {
		for i, f := range mediaFilters {
			n := p.count(f)
			if n == 0 && i > 0 {
				continue
			}
			chip := ui.Row(c.Key(i)).Gap(5).AlignItems(ui.Center).Padding(5, 11).Radius(999).Cursor(ui.CursorPointer).
				Role(ui.RoleButton).Label(fmt.Sprintf("%s, %d", f.label, n)).Focusable()
			on := p.filter == i
			if on {
				chip.Background(t.Accent)
			} else {
				chip.Border(1, t.Border).Background(t.Surface)
			}
			chip.Children(func() {
				fg := t.Text
				if on {
					fg = t.AccentText
				}
				if f.icon != nil {
					ui.Icon(c, f.icon).FontSize(13).TextColor(fg)
				}
				ui.Text(c, f.label).FontSize(12).Bold().TextColor(fg)
				ui.Text(c, fmt.Sprint(n)).FontSize(12).TextColor(fg.Alpha(0.75))
			})
			if chip.Clicked() {
				p.filter = i
			}
		}
		ui.Spacer(c)
		ui.Select(c, &p.order, sortOrders).Width(140).Label("Order")
		view := 0
		if p.list {
			view = 1
		}
		if ui.Segmented(c, &view, "Grid", "List").Label("View").Changed() {
			p.list = view == 1
		}
	})
}

// libraryID finds the library's ID of an account's timeline.
func (a *App) libraryID(username, timeline string) (int64, bool) {
	for _, acc := range a.accounts {
		if strings.EqualFold(acc.Username, username) && (acc.MediaType == timeline || timeline == "media" && (acc.MediaType == "" || acc.MediaType == "all")) {
			return acc.ID, true
		}
	}
	a.toast("This account is not in the library any more")
	return 0, false
}

func (a *App) downloadOnly(urls []string) {
	p := &a.prof
	if len(urls) == 0 {
		return
	}
	a.submit(engine.Request{Username: p.username, TimelineType: p.timeline, Mode: engine.ModeLibrary, Only: urls, Redownload: true})
}

var typeColors = map[string]ui.Color{
	"photo":        ui.RGB(59, 130, 246),
	"video":        ui.RGB(239, 68, 68),
	"animated_gif": ui.RGB(168, 85, 247),
	"gif":          ui.RGB(168, 85, 247),
	"text":         ui.RGB(16, 185, 129),
}

func typeIcon(kind string) *ui.SVG {
	switch kind {
	case "photo":
		return iconImage
	case "video":
		return iconVideo
	case "animated_gif", "gif":
		return iconGIF
	}
	return iconText
}

func (a *App) profileGrid(c *ui.Context) {
	t := c.Theme()
	p := &a.prof
	view := p.view
	p.grid.Key = func(i int) any { return p.entries[view[i]].URL + "|" + fmt.Sprint(p.entries[view[i]].TweetID) }
	p.grid.Selected = &p.gridRow
	p.grid.Selection = &p.selection
	p.grid.Label = func(i int) string { return mediaTitle(p.entries[view[i]]) }
	// Square cells: as many columns as fit cells of 150 or more, each as
	// tall as it is wide.
	const minCell, gap = 150, 8
	size := float32(minCell)
	if w := p.gridWidth; w > 0 {
		cols := max(1, int((w+gap)/(minCell+gap)))
		size = (w - gap*float32(cols-1)) / float32(cols)
	}
	grid := ui.GridView(c, &p.grid, len(view), minCell, size, func(i int) {
		e := p.entries[view[i]]
		path := p.savedPath(e)
		cell := ui.Box(c).Fill().Radius(10).Clip().Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorPointer)
		cell.Children(func() {
			if e.Type == "text" {
				ui.Text(c, html.UnescapeString(e.Content)).FontSize(11.5).Padding(30, 10, 10).MaxLines(7)
			} else if bm := a.images.get(e.Thumbnail, "", imgThumb); bm != nil {
				ui.Image(c, bm).Fill().Fit(ui.Cover)
			} else {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Icon(c, typeIcon(e.Type)).FontSize(26).TextColor(t.TextMuted.Alpha(0.5))
				})
			}
			ui.Row(c).Absolute().Top(6).Left(6).Padding(3, 5).Radius(5).Background(typeColors[e.Type]).Children(func() {
				ui.Icon(c, typeIcon(e.Type)).FontSize(12).TextColor(ui.RGB(255, 255, 255))
			})
			if e.DurationMS > 0 {
				ui.Text(c, duration(e.DurationMS)).Absolute().Bottom(6).Left(6).Padding(1, 6).Radius(5).
					Background(ui.RGBA(0, 0, 0, 0.6)).TextColor(ui.RGB(255, 255, 255)).FontSize(10.5).Bold()
			}
			if path != "" {
				ui.Row(c).Absolute().Top(6).Right(6).Size(20, 20).Radius(10).Center().Background(ui.RGB(21, 128, 61)).
					Tooltip("Downloaded").Children(func() {
					ui.Icon(c, iconBefore).FontSize(12).TextColor(ui.RGB(255, 255, 255))
				})
			}
			if e.IsRetweet {
				ui.Text(c, "RT").Absolute().Top(7).Left(34).Padding(1, 5).Radius(5).
					Background(ui.RGBA(0, 0, 0, 0.6)).TextColor(ui.RGB(255, 255, 255)).FontSize(10).Bold()
			}
		})
		if cell.Clicked() && cell.ClickModifiers() == 0 {
			p.viewer = i
		}
		cell.ContextMenu(func(m *ui.Menu) { a.mediaMenu(c, m, e) })
	}).Grow(1).Basis(0).Gap(gap).Label("Media")
	p.gridWidth = grid.Bounds().W
	if grid.Submitted() && p.gridRow >= 0 {
		p.viewer = p.gridRow
	}
}

var mediaColumns = []ui.TableColumn{
	{Title: "#", Width: 52, Align: ui.End},
	{Title: "Type", Width: 70},
	{Title: "Date", Width: 130},
	{Title: "Tweet"},
	{Title: "Size", Width: 90},
	{Title: "Status", Width: 110},
}

func (a *App) profileList(c *ui.Context) {
	t := c.Theme()
	p := &a.prof
	view := p.view
	p.rows.Selected = &p.row
	p.rows.Key = func(i int) any { return view[i] }
	table := ui.Table(c, &p.rows, mediaColumns, len(view), func(row, col int) {
		e := p.entries[view[row]]
		switch col {
		case 0:
			ui.Textf(c, "%d", row+1).FontSize(12).TextColor(t.TextMuted)
		case 1:
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, typeIcon(e.Type)).FontSize(13).TextColor(typeColors[e.Type])
				ui.Text(c, typeName(e.Type)).FontSize(12)
			})
		case 2:
			ui.Text(c, shortDate(e.Date)).FontSize(12).SingleLine()
		case 3:
			ui.Text(c, strings.ReplaceAll(html.UnescapeString(e.Content), "\n", " ")).FontSize(12).SingleLine()
		case 4:
			if e.Width > 0 {
				ui.Textf(c, "%d×%d", e.Width, e.Height).FontSize(12).TextColor(t.TextMuted)
			}
		case 5:
			if p.savedPath(e) != "" {
				itemStatus(c, engine.StatusDownloadedBefore)
			} else {
				itemStatus(c, "")
			}
		}
	}).Grow(1).Basis(0)
	if table.Submitted() && p.row >= 0 {
		p.viewer = p.row
	}
	table.ContextMenu(func(m *ui.Menu) {
		if p.row >= 0 && p.row < len(view) {
			a.mediaMenu(c, m, p.entries[view[p.row]])
		}
	})
}

func (a *App) mediaMenu(c *ui.Context, m *ui.Menu, e backend.TimelineEntry) {
	path := a.prof.savedPath(e)
	if e.Type != "text" && m.Item("Download").Chosen() {
		a.downloadOnly([]string{e.URL})
	}
	if m.Item("Open File").Disabled(path == "").Chosen() {
		a.openPath(path)
	}
	if m.Item("Show in Folder").Disabled(path == "").Chosen() {
		a.openPath(filepath.Dir(path))
	}
	m.Separator()
	if m.Item("Open Post on X").Chosen() {
		c.OpenURL(postURL(e))
	}
	if e.URL != "" && m.Item("Copy Media URL").Chosen() {
		c.WriteClipboard(e.URL)
	}
}

// mediaViewer shows the item the viewer is on over the window, with the
// post beside it.
func (a *App) mediaViewer(c *ui.Context) {
	p := &a.prof
	if p.viewer < 0 || p.viewer >= len(p.view) {
		a.stopVideo()
		return
	}
	t := c.Theme()
	e := p.entries[p.view[p.viewer]]
	step := func(d int) {
		if n := p.viewer + d; n >= 0 && n < len(p.view) {
			p.viewer = n
			p.grid.ScrollIntoView(n)
		}
	}
	ui.Overlay(c, func() {
		back := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Padding(28).AlignItems(ui.Stretch).
			Background(ui.RGBA(0, 0, 0, 0.72)).Modal()
		back.Children(func() {
			panel := ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Radius(14).
				Background(t.Surface).Clip().Role(ui.RoleDialog).Label(mediaTitle(e))
			panel.Children(func() {
				ui.Box(c).Grow(1).Basis(0).MinWidth(0).Clip().Background(ui.RGB(12, 12, 14)).Children(func() {
					a.viewerMedia(c, e)
					// Top center: a video's player has its buttons in the corners.
					ui.Row(c).Absolute().Top(12).Left(0).Right(0).Justify(ui.Center).PassThrough().Children(func() {
						ui.Text(c, fmt.Sprintf("%d / %d", p.viewer+1, len(p.view))).
							Padding(3, 9).Radius(999).Background(ui.RGBA(0, 0, 0, 0.55)).
							TextColor(ui.RGB(255, 255, 255)).FontSize(11.5).Bold()
					})
					arrow := func(ic *ui.SVG, label string, left bool, d int) {
						b := ui.Row(c.Key(label)).Absolute().Size(38, 38).Radius(19).Center().
							Background(ui.RGBA(0, 0, 0, 0.5)).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label).
							TopPercent(50).Margin(-19, 0, 0, 0)
						if left {
							b.Left(12)
						} else {
							b.Right(12)
						}
						b.Children(func() { ui.Icon(c, ic).FontSize(18).TextColor(ui.RGB(255, 255, 255)) })
						if b.Clicked() {
							step(d)
						}
					}
					if p.viewer > 0 {
						arrow(iconPrev, "Previous", true, -1)
					}
					if p.viewer < len(p.view)-1 {
						arrow(iconNext, "Next", false, 1)
					}
				})
				a.viewerDetails(c, e)
			})
			if back.OverlayShortcut(0, ui.KeyEscape) || (panel.PressedOutside() && back.Pressed()) {
				p.viewer = -1
			}
			if back.OverlayShortcut(0, ui.KeyLeft) {
				step(-1)
			}
			if back.OverlayShortcut(0, ui.KeyRight) {
				step(1)
			}
		})
	})
}

func (a *App) viewerMedia(c *ui.Context, e backend.TimelineEntry) {
	t := c.Theme()
	p := &a.prof
	switch e.Type {
	case "video", "animated_gif", "gif":
		a.playVideo(e)
		if p.video != nil {
			ui.WebView(c, p.video).Absolute().Left(0).Top(0).Right(0).Bottom(0)
			return
		}
	default:
		a.stopVideo()
	}
	if e.Type == "text" {
		ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Padding(40).Children(func() {
			ui.Text(c, html.UnescapeString(e.Content)).FontSize(16).TextColor(ui.RGB(240, 240, 240)).Selectable().MaxWidth(560)
		})
		return
	}
	local := ""
	if path := p.savedPath(e); path != "" && e.Type == "photo" {
		local = path
	}
	if bm := a.images.get(largeURL(e), local, imgPreview); bm != nil {
		ui.Image(c, bm).Absolute().Left(0).Top(0).Right(0).Bottom(0).Fit(ui.Contain)
		return
	}
	// The thumbnail, while the large picture loads.
	if bm := a.images.get(e.Thumbnail, "", imgThumb); bm != nil {
		ui.Image(c, bm).Absolute().Left(0).Top(0).Right(0).Bottom(0).Fit(ui.Contain).Opacity(0.6)
	}
	ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Center().Children(func() {
		ui.Spinner(c).TextColor(t.AccentText)
	})
}

func (a *App) viewerDetails(c *ui.Context, e backend.TimelineEntry) {
	t := c.Theme()
	p := &a.prof
	path := p.savedPath(e)
	ui.Column(c).Width(300).Shrink(0).MinHeight(0).Padding(16, 18).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Post").FontSize(15).Bold()
			ui.Spacer(c)
			b := ui.Row(c).Size(28, 28).Radius(14).Center().Cursor(ui.CursorPointer).Role(ui.RoleButton).Label("Close").Tooltip("Close (Esc)")
			b.Children(func() { ui.Icon(c, iconClose).FontSize(16) })
			if b.Clicked() {
				p.viewer = -1
			}
		})
		author, name := e.AuthorUsername, e.AuthorName
		if author == "" {
			author, name = p.username, p.info.Nick
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			pic := ""
			if strings.EqualFold(author, p.username) {
				pic = variant(p.info.AvatarVariants, "200x200", p.info.ProfileImage)
			}
			a.roundPicture(c, pic, firstOf(name, author), 38)
			ui.Column(c).Grow(1).Basis(0).Gap(1).Children(func() {
				ui.Text(c, firstOf(name, author)).FontSize(13).Bold().SingleLine()
				ui.Text(c, "@"+author).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
			})
			ui.Badge(c, typeName(e.Type)).Background(typeColors[e.Type].Alpha(0.15)).TextColor(typeColors[e.Type])
		})
		if e.IsRetweet {
			ui.Text(c, "Reposted by @"+p.username).FontSize(11.5).TextColor(t.TextMuted)
		}
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			ui.Column(c).Gap(12).Children(func() {
				if text := strings.TrimSpace(html.UnescapeString(e.Content)); text != "" {
					ui.Text(c, text).FontSize(13).Selectable()
				}
				ui.Divider(c)
				ui.Grid(c).Columns(2).GapX(12).GapY(8).Children(func() {
					stat := func(label string, n int) {
						ui.Column(c).Gap(1).Children(func() {
							ui.Text(c, compact(n)).FontSize(13).Bold()
							ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
						})
					}
					stat("Replies", e.ReplyCount)
					stat("Reposts", e.RetweetCount)
					stat("Likes", e.FavoriteCount)
					stat("Views", e.ViewCount)
					stat("Quotes", e.QuoteCount)
					stat("Bookmarks", e.BookmarkCount)
				})
				ui.Divider(c)
				detail := func(ic *ui.SVG, text string) {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, ic).FontSize(13).TextColor(t.TextMuted)
						ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).Selectable()
					})
				}
				detail(iconCalendar, shortDate(e.Date))
				if e.Width > 0 {
					detail(typeIcon(e.Type), fmt.Sprintf("%d × %d", e.Width, e.Height))
				}
				if e.DurationMS > 0 {
					detail(iconPlay, duration(e.DurationMS))
				}
				if path != "" {
					detail(iconBefore, filepath.Base(path))
				}
			})
		})
		ui.Column(c).Gap(8).Shrink(0).Children(func() {
			if e.Type != "text" {
				label := "Download " + strings.ToUpper(firstOf(e.Extension, "file"))
				if path != "" {
					label = "Download Again"
				}
				if iconButton(c, ui.PrimaryButton(c, ""), iconDownload, label).FillWidth().Clicked() {
					a.downloadOnly([]string{e.URL})
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				if path != "" {
					if iconButton(c, ui.Button(c, ""), iconFolder, "Show File").Grow(1).Basis(0).Clicked() {
						a.openPath(filepath.Dir(path))
					}
				}
				if iconButton(c, ui.Button(c, ""), iconOpen, "Open Post").Grow(1).Basis(0).Clicked() {
					c.OpenURL(postURL(e))
				}
			})
		})
	})
}

// playVideo plays e in the viewer's web view: X's videos are MP4, which
// the system's web engine plays with sound. The page is untrusted: it
// reaches nothing of the app.
func (a *App) playVideo(e backend.TimelineEntry) {
	p := &a.prof
	if p.video == nil {
		v, err := a.win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}, BackgroundColor: "#0c0c0e"})
		if err != nil {
			log.Printf("viewer: %v", err)
			return
		}
		p.video = v
	}
	src := e.URL
	if path := p.savedPath(e); path != "" {
		src = "file://" + filepath.ToSlash(path)
	}
	if p.videoURL == src {
		return
	}
	p.videoURL = src
	attrs := "controls autoplay playsinline"
	if e.Type != "video" {
		attrs = "autoplay loop muted playsinline" // a GIF
	}
	poster := html.EscapeString(largeURL(e))
	page := `<!doctype html><meta charset="utf-8"><style>html,body{margin:0;height:100%;background:#0c0c0e}` +
		`video{width:100%;height:100%;object-fit:contain;outline:none}</style>` +
		`<video ` + attrs + ` poster="` + poster + `" src="` + html.EscapeString(src) + `"></video>`
	p.video.Page().LoadHTML(page, "")
}

func (a *App) stopVideo() {
	p := &a.prof
	if p.video != nil && p.videoURL != "" {
		p.video.Page().LoadHTML(`<!doctype html><body style="margin:0;background:#0c0c0e"></body>`, "")
		p.videoURL = ""
	}
}

// roundPicture draws a profile picture in a circle of size, or the name's
// first letter while it loads or when there is none.
func (a *App) roundPicture(c *ui.Context, url, name string, size float32) {
	t := c.Theme()
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Clip().Background(t.Border).Center().Children(func() {
		if bm := a.images.get(url, "", imgAvatar); bm != nil {
			ui.Image(c, bm).Size(size, size).Fit(ui.Cover)
			return
		}
		initial := strings.ToUpper(string([]rune(strings.TrimPrefix(firstOf(name, "?"), "@"))[:1]))
		ui.Text(c, initial).FontSize(size * 0.42).Bold().TextColor(t.TextMuted)
	})
}

// iconButton puts an icon before a button's label.
func iconButton(c *ui.Context, b ui.Element, ic *ui.SVG, label string) ui.Element {
	return b.Label(label).Children(func() {
		ui.Icon(c, ic)
		ui.Text(c, label).SingleLine()
	})
}

func mediaTitle(e backend.TimelineEntry) string {
	return typeName(e.Type) + " from " + shortDate(e.Date)
}

func typeName(kind string) string {
	switch kind {
	case "photo":
		return "Image"
	case "video":
		return "Video"
	case "animated_gif", "gif":
		return "GIF"
	}
	return "Text"
}

func postURL(e backend.TimelineEntry) string {
	return fmt.Sprintf("https://x.com/i/status/%d", int64(e.TweetID))
}

// largeURL is the picture the viewer shows: X's "large" (2048 px) rather
// than "orig", which can be 4096 px and slow to decode.
func largeURL(e backend.TimelineEntry) string {
	src := e.Thumbnail
	if e.Type == "photo" {
		src = e.URL
	}
	if i := strings.Index(src, "&name="); i >= 0 {
		return src[:i] + "&name=large"
	}
	return src
}

func variant(vs []backend.ImageVariant, name, fallback string) string {
	for _, v := range vs {
		if v.Name == name {
			return v.URL
		}
	}
	return fallback
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func duration(ms int) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func joinedDate(s string) string {
	for _, layout := range []string{time.RubyDate, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("Jan 2006")
		}
	}
	return ""
}

func displayURL(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.TrimPrefix(u, "www.")
	return strings.TrimSuffix(u, "/")
}
