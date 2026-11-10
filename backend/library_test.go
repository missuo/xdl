package backend

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// isolate gives a test its own home, so the library, the settings and the
// old folder are temporary.
func isolate(t *testing.T) string {
	t.Helper()
	CloseDB()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Cleanup(CloseDB)
	if !strings.HasPrefix(GetDBPath(), home) {
		t.Skipf("the library %s is not under the test's home on %s", GetDBPath(), runtime.GOOS)
	}
	return home
}

// An XDL 2.2 library: the accounts table as the first versions made it,
// before the columns later versions added, in the old folder.
func writeOldLibrary(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(legacyDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", "file:"+filepath.Join(legacyDir(), "accounts.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	timeline := `{"account_info":{"name":"alice","nick":"Alice","followers_count":12,"statuses_count":34},"total_urls":1,
		"timeline":[{"url":"https://pbs.twimg.com/media/A?format=jpg&name=orig","date":"Tue Nov 10 11:43:42 +0000 2026","tweet_id":"7","type":"photo"}]}`
	for _, stmt := range []string{
		`CREATE TABLE accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL, name TEXT,
			profile_image TEXT, total_media INTEGER DEFAULT 0, last_fetched DATETIME, response_json TEXT)`,
		`INSERT INTO accounts (username, name, total_media, last_fetched, response_json) VALUES ('alice', 'Alice', 1, '2026-11-10 08:00:00', '` + timeline + `')`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLibraryMovesAndUpgradesAnOldOne(t *testing.T) {
	isolate(t)
	writeOldLibrary(t)
	if err := InitDB(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir(), "accounts.db")); !os.IsNotExist(err) {
		t.Error("the old library is still in the old folder")
	}
	accounts, err := GetAllAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	a := accounts[0]
	// The counts come from the timeline, as the old rows had no columns.
	if a.Username != "alice" || a.MediaType != "all" || !a.Completed || a.FollowersCount != 12 || a.StatusesCount != 34 {
		t.Errorf("account = %+v", a)
	}
	acc, err := GetAccountByUsernameAndMediaType("ALICE", "")
	if err != nil || !strings.Contains(acc.ResponseJSON, `"tweet_id":"7"`) {
		t.Errorf("by name: %+v, %v", acc, err)
	}
	// The history and subscriptions work on the moved library.
	if err := RecordDownloads([]DownloadRecord{{Key: "photo:A", TweetID: 7, Username: "alice", MediaType: "photo", Path: "/x"}}); err != nil {
		t.Fatal(err)
	}
	if n, _ := DownloadHistoryCount(); n != 1 {
		t.Errorf("history count = %d", n)
	}
}

func TestLibrarySaveExportImport(t *testing.T) {
	isolate(t)
	raw := `{"account_info":{"name":"bob","nick":"Bob","followers_count":5},"total_urls":2,"timeline":[]}`
	if err := SaveAccountWithStatus("bob", "Bob", "", 2, raw, "media", "C1", false); err != nil {
		t.Fatal(err)
	}
	// Saving again replaces the row.
	if err := SaveAccountWithStatus("bob", "Bobby", "", 3, raw, "media", "C2", true); err != nil {
		t.Fatal(err)
	}
	accounts, _ := GetAllAccounts()
	if len(accounts) != 1 || accounts[0].Name != "Bobby" || accounts[0].Cursor != "C2" || !accounts[0].Completed || accounts[0].TotalMedia != 3 {
		t.Fatalf("accounts = %+v", accounts)
	}
	path, err := ExportAccountToFile(accounts[0].ID, t.TempDir())
	if err != nil || filepath.Base(path) != "bob.json" {
		t.Fatalf("export: %s, %v", path, err)
	}
	if err := DeleteAccount(accounts[0].ID); err != nil {
		t.Fatal(err)
	}
	if handle, err := ImportAccountFromFile(path); err != nil || handle != "bob" {
		t.Fatalf("import: %q, %v", handle, err)
	}
	if acc, err := GetAccountByUsernameAndMediaType("bob", "all"); err != nil || acc.TotalMedia != 2 {
		t.Errorf("imported: %+v, %v", acc, err)
	}
}

func TestUpgradeTimelineJSON(t *testing.T) {
	old := `{"username":"carol","nick":"Carol","followers":9,"posts":3,"media_list":[{"tweet_id":"11","url":"u","date":"d","type":"photo"}]}`
	var tl TwitterResponse
	if err := json.Unmarshal([]byte(upgradeTimelineJSON(old)), &tl); err != nil {
		t.Fatal(err)
	}
	if tl.AccountInfo.Name != "carol" || tl.AccountInfo.FollowersCount != 9 || len(tl.Timeline) != 1 || tl.Timeline[0].TweetID != 11 {
		t.Errorf("upgraded = %+v", tl)
	}
	current := `{"account_info":{"name":"x"},"timeline":[]}`
	if got := upgradeTimelineJSON(current); got != current {
		t.Errorf("a current timeline changed: %s", got)
	}
}

func TestNaming(t *testing.T) {
	when := "Tue Nov 10 11:43:42 +0000 2026"
	it := MediaItem{URL: "https://pbs.twimg.com/media/HWkI-WUbEAAQ_nF?format=png&name=orig", Date: when, TweetID: 99, Type: "photo"}
	if got := fileName("", it, "alice", "Alice", 1); got != "alice_20261110_114342_99" {
		t.Errorf("default file name = %q", got)
	}
	if got := fileName("{name} {index} {media_id} {type} {unknown}", it, "alice", "A/l:ice", 2); got != "Alice 02 HWkI-WUbEAAQ_nF photo {unknown}" {
		t.Errorf("template = %q", got)
	}
	if got := folderName("{handle}-{date}", "alice", "Alice", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); got != "alice-20260102" {
		t.Errorf("folder = %q", got)
	}
	for url, want := range map[string]string{
		"https://pbs.twimg.com/media/A?format=png&name=orig": ".png",
		"https://pbs.twimg.com/media/A.jpg":                  ".jpg",
		"https://example.com/x":                              ".jpg",
	} {
		if got := fileExtension(url, "photo"); got != want {
			t.Errorf("extension of %s = %q", url, got)
		}
	}
	if fileExtension("https://video.twimg.com/x/y.mp4?tag=1", "animated_gif") != ".mp4" || fileExtension("", "text") != ".txt" {
		t.Error("video or text extension")
	}
	for url, want := range map[string]string{
		"https://pbs.twimg.com/media/HWkI-WUbEAAQ_nF?format=jpg&name=orig":          "HWkI-WUbEAAQ_nF",
		"https://video.twimg.com/tweet_video/GbXyZ.mp4":                             "GbXyZ",
		"https://video.twimg.com/ext_tw_video/1/pu/vid/avc1/720x1280/AbCdEfGh1.mp4": "AbCdEfGh1",
		"https://video.twimg.com/ext_tw_video/1/pu/vid/avc1/720x1280/1234.mp4":      "",
		"https://example.com/media/x.jpg":                                           "",
	} {
		if got := ExtractOriginalFilename(url); got != want {
			t.Errorf("original name of %s = %q, want %q", url, got, want)
		}
	}
	for in, want := range map[string]string{`a/b:c*?"<>|d`: "abcd", "CON": "CON_", "lpt1.txt": "lpt1.txt_", "x. ": "x", "COM0": "COM0"} {
		if got := sanitizeFilenamePart(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanPathsNumbersCollisions(t *testing.T) {
	when := "Tue Nov 10 11:43:42 +0000 2026"
	items := []MediaItem{
		{URL: "https://pbs.twimg.com/media/A?format=jpg", Date: when, TweetID: 1, Type: "photo"},
		{URL: "https://pbs.twimg.com/media/B?format=jpg", Date: when, TweetID: 1, Type: "photo"},
		{URL: "https://video.twimg.com/v.mp4", Date: when, TweetID: 2, Type: "video", Username: "bob"},
		{Date: when, TweetID: 3, Type: "text"},
	}
	paths := planPaths(items, "/d", "alice", DownloadOptions{}, time.Now())
	want := []string{
		"/d/alice/images/alice_20261110_114342_1_01.jpg",
		"/d/alice/images/alice_20261110_114342_1_02.jpg",
		"/d/bob/videos/bob_20261110_114342_2.mp4",
		"/d/alice/texts/alice_20261110_114342_3.txt",
	}
	for i := range want {
		if filepath.ToSlash(paths[i]) != want[i] {
			t.Errorf("path %d = %s, want %s", i, paths[i], want[i])
		}
	}
	// {index} tells them apart: no numbering.
	paths = planPaths(items[:2], "/d", "alice", DownloadOptions{FilenameTemplate: "{tweet_id}_{index}"}, time.Now())
	if filepath.Base(paths[0]) != "1_01.jpg" || filepath.Base(paths[1]) != "1_02.jpg" {
		t.Errorf("indexed = %v", paths)
	}
}

func TestDownloadItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("picture " + r.URL.Path))
	}))
	defer srv.Close()
	dir := t.TempDir()
	when := "Tue Nov 10 11:43:42 +0000 2026"
	items := []MediaItem{
		{URL: srv.URL + "/a.jpg", Date: when, TweetID: 1, Type: "photo"},
		{URL: srv.URL + "/missing", Date: when, TweetID: 2, Type: "photo"},
		{Date: when, TweetID: 3, Type: "text", Content: "hello"},
	}
	var mu sync.Mutex
	statuses := map[int]string{}
	ev := DownloadEvents{Item: func(i int, status, _ string) {
		mu.Lock()
		statuses[i] = status
		mu.Unlock()
	}}
	opts := DownloadOptions{SkipExistingFiles: true, DeleteIncompleteFiles: true}
	counts, err := DownloadItems(context.Background(), items, dir, "alice", opts, "", ev)
	if err != nil || counts != (DownloadCounts{Downloaded: 2, Failed: 1}) {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
	if statuses[0] != StatusSuccess || statuses[1] != StatusFailed || statuses[2] != StatusSuccess {
		t.Errorf("statuses = %v", statuses)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "alice", "texts", "alice_20261110_114342_3.txt")); string(data) != "hello" {
		t.Errorf("text = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "alice", "images", "alice_20261110_114342_1.jpg")); string(data) != "picture /a.jpg" {
		t.Errorf("photo = %q", data)
	}
	// Again: what is on disk is skipped.
	counts, _ = DownloadItems(context.Background(), items[:1], dir, "alice", opts, "", DownloadEvents{})
	if counts != (DownloadCounts{Skipped: 1}) {
		t.Errorf("second run = %+v", counts)
	}
	// A stopped download reports the rest as cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	counts, err = DownloadItems(ctx, items, t.TempDir(), "alice", opts, "", DownloadEvents{})
	if err == nil || counts != (DownloadCounts{}) {
		t.Errorf("cancelled = %+v, %v", counts, err)
	}
}

func TestProxySetting(t *testing.T) {
	if _, err := CreateHTTPClient("not a url", 0); err == nil {
		t.Error("a bad proxy was taken")
	}
	f, err := proxyFunc("http://a:1, http://b:2")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "https://x.com", nil)
	first, _ := f(req)
	second, _ := f(req)
	if first.Host != "a:1" || second.Host != "b:2" {
		t.Errorf("proxies in turn = %v, %v", first, second)
	}
}

func TestTools(t *testing.T) {
	for _, tool := range Tools {
		if tool.InstallCommand() == "" || tool.Title == "" {
			t.Errorf("%s has no install command", tool.Name)
		}
	}
	if _, err := tokenSlot("other"); err == nil {
		t.Error("an unknown token slot was taken")
	}
}
