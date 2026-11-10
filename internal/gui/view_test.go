package gui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/server"
	"github.com/missuo/xdl/internal/settings"
)

func testApp(t *testing.T) *App {
	store := settings.OpenPath(filepath.Join(t.TempDir(), "settings.json"))
	eng := engine.New(store)
	sched := schedule.New(eng)
	return &App{
		eng: eng, store: store, sched: sched, cfg: store.Get(),
		srv:    server.New(eng, sched, "127.0.0.1:47399", "test", "gui"),
		page:   pageDownload,
		form:   downloadForm{label: "Media"},
		states: map[string]engine.State{}, itemRow: -1, jobRow: -1, libRow: -1, subRow: -1,
		subForm: subscriptionForm{timeline: "Media", interval: "Every day"},
		tools:   map[string]toolInfo{},
		avatars: map[string]*ui.Bitmap{}, fetching: map[string]bool{},
	}
}

// Every page and settings tab builds, with and without data.
func TestPagesRender(t *testing.T) {
	a := testApp(t)
	now := time.Now()
	a.jobs = []engine.Job{
		{ID: "1", Username: "nasa", TimelineType: "media", Mode: engine.ModeDownload, Source: "extension",
			State: engine.StateDownloading, Total: 10, Current: 4, Percent: 40, CreatedAt: now},
		{ID: "2", Username: "esa", TimelineType: "likes", Mode: engine.ModeFetch, Source: "schedule",
			State: engine.StateFailed, Error: "boom", CreatedAt: now},
	}
	a.focusJob, a.itemsJob = "1", "1"
	a.items = []engine.Item{
		{TimelineEntry: backend.TimelineEntry{URL: "https://pbs.twimg.com/media/a.jpg", Type: "photo", Date: "Tue Nov 03 13:18:00 +0000 2026", Content: "Q&amp;A"}, Status: "success"},
		{TimelineEntry: backend.TimelineEntry{Type: "video"}, Status: engine.StatusDownloadedBefore},
	}
	a.accounts = []backend.AccountListItem{{ID: 1, Username: "nasa", Name: "NASA", MediaType: "media", TotalMedia: 10, Completed: true}}
	a.subs = []backend.Subscription{{ID: 1, Username: "nasa", TimelineType: "media", IntervalMins: 1440, Enabled: true, NextRun: now.Add(time.Hour)}}

	want := map[string]string{
		pageDownload: "NASA", pageQueue: "@esa (likes)", pageSubs: "Every day", pageLibrary: "@nasa", pageLogs: "Logs",
	}
	for page, text := range want {
		a.page = page
		tt := ui.NewTester(a.view, 1040, 720)
		if !tt.HasText(text) && !tt.HasText(page) {
			t.Errorf("page %s: no %q in %q", page, text, tt.Texts())
		}
	}
	a.page = pageSettings
	for tab, text := range []string{"X Account", "Download History", "FFmpeg", "AI agents (MCP)"} {
		a.settingsTab = tab
		if tt := ui.NewTester(a.view, 1040, 720); !tt.HasText(text) {
			t.Errorf("settings tab %d: no %q in %q", tab, text, tt.Texts())
		}
	}
	if got := shortDate("Tue Nov 03 13:18:00 +0000 2026"); len(got) != 16 || got[:4] != "2026" {
		t.Errorf("shortDate = %q", got)
	}
}
