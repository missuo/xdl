// Package gui is XDL's desktop app: a window of native UI drawn by MyGo and
// a menu bar icon, around the same engine and local API the CLI uses.
package gui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/deps"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/logbuf"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/server"
	"github.com/missuo/xdl/internal/settings"
	"github.com/missuo/xdl/internal/version"
)

const (
	pageDownload = "download"
	pageQueue    = "queue"
	pageLibrary  = "library"
	pageSettings = "settings"
	pageSubs     = "subscriptions"
	pageLogs     = "logs"
)

// App is the state the window shows. It is read and changed on the main
// thread only: other goroutines go through win.Update or mygo.RunOnMain.
type App struct {
	eng    *engine.Engine
	store  *settings.Store
	srv    *server.Server
	sched  *schedule.Scheduler
	apiErr string

	win          *mygo.Window
	tray         *mygo.Tray
	trayKey      string
	trayAnim     trayAnimator
	trayFinished bool // a job finished since the last updateTray
	quitting     bool

	page string
	cfg  settings.Settings // the settings being edited

	// Download page.
	form      downloadForm
	handle    string
	focusJob  string // the job the Download page shows
	items     []engine.Item
	itemsJob  string
	itemsList ui.ListState
	itemRow   int

	// Every job, newest first, and their last known states.
	jobs     []engine.Job
	states   map[string]engine.State
	jobsList ui.ListState
	jobRow   int

	// Library page.
	accounts   []backend.AccountListItem
	libList    ui.ListState
	libRow     int
	libFilter  string
	lib        libraryState
	libLoading bool
	gifBusy    bool

	// Settings page.
	token          string
	tokenSaved     bool
	historyCount   int
	confirmHistory bool
	concurrent     float64
	retries        float64
	timeout        float64

	settingsTab int

	// Subscriptions page.
	subs       []backend.Subscription
	subsList   ui.ListState
	subRow     int
	subForm    subscriptionForm
	confirmSub bool

	// Dependencies (in Settings).
	depStatus   map[string]deps.Status
	depChecked  bool
	depChecking bool
	depProgress map[string]float64 // in flight: 0…1, or -1 when the size is unknown
	depError    map[string]string

	// Logs page.
	logs   []string
	logSeq uint64

	avatars  map[string]*ui.Bitmap
	fetching map[string]bool
}

// Run starts the desktop app and returns when it quits.
func Run() error {
	logbuf.Install("")
	mygo.App.SetName("XDL")
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running instance comes to the front
	}
	if err := backend.InitDB(); err != nil {
		log.Printf("library: %v", err)
	}

	store := settings.Open()
	a := &App{
		eng:         engine.New(store),
		store:       store,
		page:        pageDownload,
		form:        downloadForm{label: "Media"},
		cfg:         store.Get(),
		states:      map[string]engine.State{},
		itemRow:     -1,
		jobRow:      -1,
		libRow:      -1,
		subRow:      -1,
		subForm:     subscriptionForm{timeline: "Media", interval: "Every day"},
		depStatus:   map[string]deps.Status{},
		depProgress: map[string]float64{},
		depError:    map[string]string{},
		avatars:     map[string]*ui.Bitmap{},
		fetching:    map[string]bool{},
	}
	a.loadNumbers()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go a.eng.Run(ctx)
	a.sched = schedule.New(a.eng)
	a.srv = server.New(a.eng, a.sched, "", version.Version, "gui")
	go func() {
		err := a.srv.Listen()
		if err == nil {
			// Only the process holding the API port runs subscriptions.
			go a.sched.Run(ctx)
			err = a.srv.Serve(ctx)
		}
		if err != nil {
			log.Printf("api: %v", err)
			mygo.RunOnMain(func() {
				a.apiErr = err.Error()
				if a.win != nil {
					a.win.Invalidate()
				}
			})
		}
	}()

	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.show()
		}
	})
	// Closing the window keeps XDL in the menu bar, so the extension and
	// the CLI can still hand it jobs.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		stop()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.WhenReady(a.ready)
	err := mygo.App.Run()
	backend.CloseDB()
	return err
}

func (a *App) ready() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:         "XDL",
		Width:         1040,
		Height:        720,
		MinWidth:      820,
		MinHeight:     560,
		StateKey:      "main",
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.hide()
		}
	})

	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(lookIdle, 0, false),
		IconIsTemplate: trayTemplate(),
		ToolTip:        "XDL",
	})
	if err != nil {
		log.Printf("menu bar icon: %v", err)
	} else {
		a.tray = tray
		tray.OnAppearanceChange(func() {
			a.trayAnim.shown = ""
			a.drawTrayIcon()
		})
		a.updateTray()
	}

	events, _ := a.eng.Subscribe()
	go a.pump(events)
	go a.loadAccounts()
	go a.loadSubscriptions()
	go a.watchLogs()
}

func (a *App) show() {
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
	}
	a.win.Show()
	a.win.Restore()
	a.win.Focus()
	mygo.App.Focus()
}

// hide closes the window to the menu bar, and takes the Dock icon away
// with it.
func (a *App) hide() {
	a.win.Hide()
	if runtime.GOOS == "darwin" && a.tray != nil {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
}

// pump hands engine events to the main thread in batches, so a download
// reporting thousands of items redraws ten times a second, not thousands.
func (a *App) pump(events <-chan engine.Event) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var pending []engine.Event
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			pending = append(pending, ev)
		case <-tick.C:
			if len(pending) == 0 {
				continue
			}
			batch := pending
			pending = nil
			a.win.Update(func() { a.apply(batch) })
		}
	}
}

func (a *App) apply(batch []engine.Event) {
	library, subs := false, false
	for _, ev := range batch {
		switch ev.Type {
		case engine.EventJob:
			a.applyJob(*ev.Job, ev.Removed)
		case engine.EventItem:
			it := ev.Item
			if it.JobID == a.itemsJob && it.Index >= 0 && it.Index < len(a.items) {
				a.items[it.Index].Status, a.items[it.Index].Path = it.Status, it.Path
			}
		case engine.EventLibrary:
			library = true
		case engine.EventSubscriptions:
			subs = true
		}
	}
	if library {
		go a.loadAccounts()
	}
	if subs {
		go a.loadSubscriptions()
	}
	a.updateTray()
}

func (a *App) applyJob(j engine.Job, removed bool) {
	i := slices.IndexFunc(a.jobs, func(x engine.Job) bool { return x.ID == j.ID })
	if removed {
		if i >= 0 {
			a.jobs = slices.Delete(a.jobs, i, i+1)
		}
		delete(a.states, j.ID)
		return
	}
	if i >= 0 {
		a.jobs[i] = j
	} else {
		a.jobs = slices.Insert(a.jobs, 0, j)
	}
	prev, seen := a.states[j.ID]
	a.states[j.ID] = j.State
	if prev == j.State && seen {
		return
	}

	// A job from elsewhere becomes the Download page's when it starts and
	// that page shows nothing running.
	if a.focusJob == "" || (j.State == engine.StateFetching && a.focusedFinished()) {
		a.focusJob = j.ID
	}
	if j.ID == a.focusJob && (j.State == engine.StateDownloading || j.State.Finished()) {
		a.loadItems(j.ID)
	}

	if !seen && j.State == engine.StateQueued && j.Source == "extension" {
		a.notify(j.ID+"-queued", "XDL", fmt.Sprintf("Queued %s from the browser", j.Label()))
	}
	if j.State.Finished() && seen {
		if j.State == engine.StateDone && prev != engine.StateQueued {
			a.trayFinished = true
		}
		a.notifyFinished(j)
	}
}

func (a *App) focusedFinished() bool {
	for _, j := range a.jobs {
		if j.ID == a.focusJob {
			return j.State.Finished()
		}
	}
	return true
}

func (a *App) focused() (engine.Job, bool) {
	for _, j := range a.jobs {
		if j.ID == a.focusJob {
			return j, true
		}
	}
	return engine.Job{}, false
}

func (a *App) loadItems(id string) {
	items, ok := a.eng.Items(id)
	if !ok {
		return
	}
	if a.itemsJob != id {
		a.itemRow = -1
	}
	a.items, a.itemsJob = items, id
}

func (a *App) notifyFinished(j engine.Job) {
	if !a.cfg.Notify {
		return
	}
	// Jobs started from the window need no banner while it is in front.
	if j.Source == "gui" && a.win.IsVisible() && mygo.App.IsActive() {
		return
	}
	switch j.State {
	case engine.StateDone:
		title := j.Label() + " downloaded"
		if j.Mode == engine.ModeFetch {
			title = j.Label() + " fetched"
		}
		a.notify(j.ID, title, j.Message)
	case engine.StateFailed:
		a.notify(j.ID, j.Label()+" failed", j.Error)
	}
}

func (a *App) notify(id, title, body string) {
	if !a.cfg.Notify {
		return
	}
	go func() {
		n := mygo.NewNotification(mygo.NotificationOptions{ID: "xdl-" + id, Title: title, Body: body})
		n.OnClick(func() { mygo.RunOnMain(func() { a.page = pageQueue; a.show() }) })
		if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
			log.Printf("notification: %v", err)
		}
	}()
}

// submit queues a job from the window and shows it on the Download page.
func (a *App) submit(req engine.Request) {
	req.Source = "gui"
	j, err := a.eng.Submit(req)
	if err != nil && !errors.Is(err, engine.ErrDuplicate) {
		a.toast(err.Error())
		return
	}
	a.applyJob(j, false)
	a.focusJob = j.ID
	a.items, a.itemsJob = nil, ""
	a.page = pageDownload
	if errors.Is(err, engine.ErrDuplicate) {
		a.toast(j.Label() + " is already in the queue")
	}
}

var toastQueue []string

// toast shows a message at the bottom of the window on its next frame.
func (a *App) toast(msg string) {
	toastQueue = append(toastQueue, msg)
	a.win.Invalidate()
}

func (a *App) downloadDir() string {
	return a.cfg.DownloadDir()
}

func (a *App) openPath(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		a.toast("Not found: " + path)
		return
	}
	go func() {
		if err := mygo.Shell.OpenPath(path); err != nil {
			log.Printf("open %s: %v", path, err)
		}
	}()
}

func (a *App) loadAccounts() {
	accounts, err := backend.GetAllAccounts()
	if err != nil {
		log.Printf("library: %v", err)
		return
	}
	a.win.Update(func() {
		a.accounts = accounts
		if a.libRow >= len(accounts) {
			a.libRow = -1
		}
	})
}

func (a *App) watchLogs() {
	var last uint64
	for {
		if seq := logbuf.Default.Seq(); seq != last {
			lines, seq := logbuf.Default.Lines()
			last = seq
			a.win.Update(func() { a.logs, a.logSeq = lines, seq })
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (a *App) loadNumbers() {
	a.concurrent = float64(a.cfg.ConcurrentDownloads)
	a.retries = float64(a.cfg.RetryAttempts)
	a.timeout = float64(a.cfg.FetchTimeout)
}

// saveSettings writes the settings being edited.
func (a *App) saveSettings() {
	a.cfg.ConcurrentDownloads = int(a.concurrent)
	a.cfg.RetryAttempts = int(a.retries)
	a.cfg.FetchTimeout = int(a.timeout)
	if err := a.store.Set(a.cfg); err != nil {
		a.toast("Saving settings failed: " + err.Error())
	}
	a.cfg = a.store.Get()
}

// avatar returns the account picture for url, fetching it the first time.
func (a *App) avatar(url string) *ui.Bitmap {
	if url == "" {
		return nil
	}
	if bm, ok := a.avatars[url]; ok {
		return bm
	}
	if !a.fetching[url] {
		a.fetching[url] = true
		go func() {
			bm := fetchAvatar(url, a.cfg.Proxy)
			a.win.Update(func() { a.avatars[url] = bm })
		}()
	}
	return nil
}
