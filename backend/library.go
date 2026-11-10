package backend

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// The library: one SQLite database with the timelines XDL fetched (table
// accounts, one row per account and timeline, the timeline as JSON), the
// download history and the subscriptions. The app, the CLI and `xdl serve`
// share it, so it runs in WAL mode with a busy timeout.

// Dir is XDL's data directory: the library, the settings and the API
// details (~/Library/Application Support/XDL on macOS).
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "XDL")
}

// GetDBPath is the library's database file.
func GetDBPath() string { return filepath.Join(Dir(), "library.db") }

// legacyDir is where XDL 2.2 and earlier kept the library, a name they had
// from the app XDL began as.
func legacyDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".twitterxmediabatchdownloader")
}

var (
	db     *sql.DB
	dbMu   sync.Mutex
	dbOpen bool
)

// InitDB opens the library, moving it from where older versions kept it.
func InitDB() error {
	removeXtractor()
	return ensureDB()
}

func ensureDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbOpen {
		return nil
	}
	path := GetDBPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := adoptLegacyLibrary(path); err != nil {
		log.Printf("library: moving the library of an older version: %v", err)
	}
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	if err := prepareSchema(conn); err != nil {
		conn.Close()
		return err
	}
	db, dbOpen = conn, true
	return nil
}

// CloseDB closes the library.
func CloseDB() {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		db.Close()
	}
	db, dbOpen = nil, false
}

// adoptLegacyLibrary moves the database of XDL 2.2 and earlier to path,
// once: it folds the database's write-ahead log into it first, so the one
// file is the whole library.
func adoptLegacyLibrary(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	old := filepath.Join(legacyDir(), "accounts.db")
	if _, err := os.Stat(old); err != nil {
		return nil
	}
	if conn, err := sql.Open("sqlite", "file:"+old+"?_pragma=busy_timeout(5000)"); err == nil {
		_, _ = conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		conn.Close()
	}
	if err := moveFile(old, path); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(old + suffix)
	}
	log.Printf("library: moved %s to %s", old, path)
	return nil
}

func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	// Across volumes: copy, then remove.
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := to + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		return err
	}
	return os.Remove(from)
}

// schemaVersion counts the changes to the accounts table; libraries of
// older versions get the columns they lack.
const schemaVersion = 7

const accountsSchema = `
CREATE TABLE IF NOT EXISTS accounts (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	username        TEXT NOT NULL,
	name            TEXT,
	profile_image   TEXT,
	total_media     INTEGER DEFAULT 0,
	last_fetched    DATETIME,
	response_json   TEXT,
	group_name      TEXT DEFAULT '',
	group_color     TEXT DEFAULT '',
	media_type      TEXT DEFAULT 'all',
	cursor          TEXT DEFAULT '',
	completed       INTEGER DEFAULT 1,
	followers_count INTEGER DEFAULT 0,
	statuses_count  INTEGER DEFAULT 0,
	UNIQUE(username, media_type)
)`

var accountColumns = []struct{ name, decl string }{
	{"group_name", "TEXT DEFAULT ''"},
	{"group_color", "TEXT DEFAULT ''"},
	{"media_type", "TEXT DEFAULT 'all'"},
	{"cursor", "TEXT DEFAULT ''"},
	{"completed", "INTEGER DEFAULT 1"},
	{"followers_count", "INTEGER DEFAULT 0"},
	{"statuses_count", "INTEGER DEFAULT 0"},
}

func prepareSchema(conn *sql.DB) error {
	for _, stmt := range []string{accountsSchema, historySchema, subscriptionSchema} {
		if _, err := conn.Exec(stmt); err != nil {
			return err
		}
	}
	var version int
	if err := conn.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= schemaVersion {
		return nil
	}
	have := map[string]bool{}
	rows, err := conn.Query("PRAGMA table_info(accounts)")
	if err != nil {
		return err
	}
	for rows.Next() {
		var (
			cid          int
			name, kind   string
			notNull, pk  int
			defaultValue sql.NullString
		)
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err == nil {
			have[name] = true
		}
	}
	rows.Close()
	for _, col := range accountColumns {
		if !have[col.name] {
			if _, err := conn.Exec("ALTER TABLE accounts ADD COLUMN " + col.name + " " + col.decl); err != nil {
				return fmt.Errorf("adding %s to the library: %w", col.name, err)
			}
		}
	}
	_, _ = conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_username_media_type ON accounts(username, media_type)")
	if err := fillCounts(conn); err != nil {
		return err
	}
	_, err = conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

// fillCounts sets the follower and post counts of rows saved before the
// library had columns for them.
func fillCounts(conn *sql.DB) error {
	rows, err := conn.Query(`SELECT id, response_json FROM accounts
		WHERE COALESCE(response_json, '') != '' AND (COALESCE(followers_count, 0) = 0 OR COALESCE(statuses_count, 0) = 0)`)
	if err != nil {
		return err
	}
	type counts struct {
		id               int64
		followers, posts int
	}
	var todo []counts
	for rows.Next() {
		var c counts
		var raw string
		if rows.Scan(&c.id, &raw) == nil {
			c.followers, c.posts = countsOf(raw)
			todo = append(todo, c)
		}
	}
	rows.Close()
	for _, c := range todo {
		if _, err := conn.Exec(`UPDATE accounts SET followers_count = ?, statuses_count = ? WHERE id = ?`, c.followers, c.posts, c.id); err != nil {
			return err
		}
	}
	return nil
}

// countsOf reads an account's follower and post counts from its saved
// timeline, in the current format or the oldest one.
func countsOf(raw string) (followers, posts int) {
	var doc struct {
		AccountInfo struct {
			Followers int `json:"followers_count"`
			Posts     int `json:"statuses_count"`
		} `json:"account_info"`
		Followers int `json:"followers"`
		Posts     int `json:"posts"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return 0, 0
	}
	return max(doc.AccountInfo.Followers, doc.Followers), max(doc.AccountInfo.Posts, doc.Posts)
}

// AccountDB is a saved timeline with its JSON.
type AccountDB struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	ProfileImage string    `json:"profile_image"`
	TotalMedia   int       `json:"total_media"`
	LastFetched  time.Time `json:"last_fetched"`
	ResponseJSON string    `json:"response_json"`
	MediaType    string    `json:"media_type"`
	Cursor       string    `json:"cursor"`
	Completed    bool      `json:"completed"`
}

// AccountListItem is a saved timeline as the library lists it.
type AccountListItem struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	Name           string `json:"name"`
	ProfileImage   string `json:"profile_image"`
	TotalMedia     int    `json:"total_media"`
	LastFetched    string `json:"last_fetched"`
	MediaType      string `json:"media_type"`
	Cursor         string `json:"cursor"`
	Completed      bool   `json:"completed"`
	FollowersCount int    `json:"followers_count"`
	StatusesCount  int    `json:"statuses_count"`
}

// timelineKind is how the library names a timeline: "all" for an account's
// media, as older versions did.
func timelineKind(t string) string {
	if t == "" {
		return "all"
	}
	return t
}

// SaveAccountWithStatus saves an account's timeline, replacing the copy the
// library had.
func SaveAccountWithStatus(username, name, profileImage string, totalMedia int, responseJSON, mediaType, cursor string, completed bool) error {
	if err := ensureDB(); err != nil {
		return err
	}
	followers, posts := countsOf(responseJSON)
	_, err := db.Exec(`
		INSERT INTO accounts (username, name, profile_image, total_media, last_fetched, response_json,
			media_type, cursor, completed, followers_count, statuses_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(username, media_type) DO UPDATE SET
			name = excluded.name, profile_image = excluded.profile_image, total_media = excluded.total_media,
			last_fetched = excluded.last_fetched, response_json = excluded.response_json, cursor = excluded.cursor,
			completed = excluded.completed, followers_count = excluded.followers_count,
			statuses_count = excluded.statuses_count`,
		username, name, profileImage, totalMedia, time.Now(), responseJSON,
		timelineKind(mediaType), cursor, completed, followers, posts)
	return err
}

// GetAllAccounts lists the library, the most recently fetched first.
func GetAllAccounts() ([]AccountListItem, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT id, username, COALESCE(name, ''), COALESCE(profile_image, ''), COALESCE(total_media, 0),
			last_fetched, COALESCE(media_type, 'all'), COALESCE(cursor, ''), COALESCE(completed, 1),
			COALESCE(followers_count, 0), COALESCE(statuses_count, 0)
		FROM accounts ORDER BY last_fetched DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountListItem
	for rows.Next() {
		var a AccountListItem
		var fetched sql.NullTime
		if err := rows.Scan(&a.ID, &a.Username, &a.Name, &a.ProfileImage, &a.TotalMedia, &fetched,
			&a.MediaType, &a.Cursor, &a.Completed, &a.FollowersCount, &a.StatusesCount); err != nil {
			log.Printf("library: skipping a row: %v", err)
			continue
		}
		if fetched.Valid {
			a.LastFetched = fetched.Time.Local().Format("2006-01-02 15:04")
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const accountRow = `SELECT id, username, COALESCE(name, ''), COALESCE(profile_image, ''), COALESCE(total_media, 0),
	last_fetched, COALESCE(response_json, ''), COALESCE(media_type, 'all'), COALESCE(cursor, ''), COALESCE(completed, 1)
	FROM accounts `

func scanAccount(row *sql.Row) (*AccountDB, error) {
	var a AccountDB
	var fetched sql.NullTime
	if err := row.Scan(&a.ID, &a.Username, &a.Name, &a.ProfileImage, &a.TotalMedia, &fetched,
		&a.ResponseJSON, &a.MediaType, &a.Cursor, &a.Completed); err != nil {
		return nil, err
	}
	a.LastFetched = fetched.Time
	a.ResponseJSON = upgradeTimelineJSON(a.ResponseJSON)
	return &a, nil
}

// GetAccountByID returns a saved timeline.
func GetAccountByID(id int64) (*AccountDB, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	return scanAccount(db.QueryRow(accountRow+`WHERE id = ?`, id))
}

// GetAccountByUsernameAndMediaType returns an account's saved timeline.
func GetAccountByUsernameAndMediaType(username, mediaType string) (*AccountDB, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	return scanAccount(db.QueryRow(accountRow+`WHERE lower(username) = lower(?) AND COALESCE(media_type, 'all') = ?`,
		username, timelineKind(mediaType)))
}

// DeleteAccount removes a saved timeline (not its files).
func DeleteAccount(id int64) error {
	if err := ensureDB(); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	return err
}

// upgradeTimelineJSON turns a timeline saved in the oldest format (a
// media_list of URLs) into the current one, and leaves others as they are.
func upgradeTimelineJSON(raw string) string {
	var old struct {
		Username     *string         `json:"username"`
		Nick         string          `json:"nick"`
		Followers    int             `json:"followers"`
		Following    int             `json:"following"`
		Posts        int             `json:"posts"`
		ProfileImage string          `json:"profile_image"`
		AccountInfo  json.RawMessage `json:"account_info"`
		MediaList    []struct {
			TweetID TweetIDString `json:"tweet_id"`
			URL     string        `json:"url"`
			Date    string        `json:"date"`
			Type    string        `json:"type"`
		} `json:"media_list"`
	}
	if json.Unmarshal([]byte(raw), &old) != nil || old.AccountInfo != nil || old.Username == nil || old.MediaList == nil {
		return raw
	}
	t := TwitterResponse{
		AccountInfo: AccountInfo{
			Name: *old.Username, Nick: old.Nick, ProfileImage: old.ProfileImage,
			FollowersCount: old.Followers, FriendsCount: old.Following, StatusesCount: old.Posts,
		},
		TotalURLs: len(old.MediaList),
		Timeline:  make([]TimelineEntry, 0, len(old.MediaList)),
		Metadata:  ExtractMetadata{NewEntries: len(old.MediaList)},
	}
	for _, m := range old.MediaList {
		t.Timeline = append(t.Timeline, TimelineEntry{URL: m.URL, Date: m.Date, TweetID: m.TweetID, Type: m.Type})
	}
	b, err := json.Marshal(t)
	if err != nil {
		return raw
	}
	return string(b)
}

// ExportAccountToFile writes a saved timeline to dir as <handle>.json, and
// returns its path.
func ExportAccountToFile(id int64, dir string) (string, error) {
	acc, err := GetAccountByID(id)
	if err != nil {
		return "", err
	}
	name := sanitizeFilenamePart(firstNonEmpty(acc.Username, acc.Name, "timeline"))
	if t := timelineKind(acc.MediaType); t != "all" && t != "media" {
		name += " (" + t + ")"
	}
	path := filepath.Join(dir, name+".json")
	return path, os.WriteFile(path, []byte(acc.ResponseJSON), 0o644)
}

// ImportAccountFromFile adds a timeline an export wrote to the library, and
// returns its account's handle.
func ImportAccountFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	raw := upgradeTimelineJSON(string(data))
	var t TwitterResponse
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return "", fmt.Errorf("not an XDL timeline: %w", err)
	}
	handle := strings.TrimSpace(t.AccountInfo.Name)
	if handle == "" {
		return "", errors.New("not an XDL timeline: it names no account")
	}
	total := t.TotalURLs
	if total == 0 {
		total = len(t.Timeline)
	}
	return handle, SaveAccountWithStatus(handle, t.AccountInfo.Nick, t.AccountInfo.ProfileImage, total, raw, "all", "", true)
}
