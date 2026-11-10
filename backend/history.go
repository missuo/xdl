package backend

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The download history remembers every media item XDL has downloaded, by
// an identity that survives renamed templates, moved folders and other
// video qualities, so a job can skip what was downloaded before without a
// single request.

const historySchema = `
CREATE TABLE IF NOT EXISTS download_history (
	media_key     TEXT PRIMARY KEY,
	tweet_id      INTEGER,
	username      TEXT,
	media_type    TEXT,
	url           TEXT,
	path          TEXT,
	downloaded_at DATETIME
)`

var numericID = regexp.MustCompile(`^[0-9]+$`)

// MediaKey identifies a media item independently of its size or quality:
//
//	pbs.twimg.com/media/<id>.jpg?name=orig        photo:<id>
//	video.twimg.com/amplify_video/<id>/…/x.mp4    video:<id>
//	video.twimg.com/ext_tw_video/<id>/…/x.mp4     video:<id>
//	video.twimg.com/tweet_video/<name>.mp4        gif:<name>
//	a text tweet                                  text:<tweet id>
func MediaKey(mediaURL string, tweetID int64, mediaType string) string {
	if mediaType == "text" {
		return "text:" + strconv.FormatInt(tweetID, 10)
	}
	u, err := url.Parse(mediaURL)
	if err != nil || u.Host == "" {
		return "url:" + mediaURL
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	base := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	switch {
	case strings.HasSuffix(u.Host, "pbs.twimg.com") && len(segs) >= 2 && segs[0] == "media":
		return "photo:" + base
	case strings.HasSuffix(u.Host, "video.twimg.com") && len(segs) >= 2 && segs[0] == "tweet_video":
		return "gif:" + base
	case strings.HasSuffix(u.Host, "video.twimg.com") && len(segs) >= 2 && numericID.MatchString(segs[1]):
		return "video:" + segs[1]
	}
	return "url:" + u.Host + u.Path
}

// DownloadRecord is one downloaded item.
type DownloadRecord struct {
	Key       string
	TweetID   int64
	Username  string
	MediaType string
	URL       string
	Path      string
}

// DownloadedPaths returns, for the keys the history holds, where they were
// saved.
func DownloadedPaths(keys []string) (map[string]string, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	out := make(map[string]string)
	const chunk = 500
	for start := 0; start < len(keys); start += chunk {
		part := keys[start:min(start+chunk, len(keys))]
		args := make([]any, len(part))
		for i, k := range part {
			args[i] = k
		}
		rows, err := db.Query(`SELECT media_key, path FROM download_history WHERE media_key IN (?`+
			strings.Repeat(",?", len(part)-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, p string
			if err := rows.Scan(&key, &p); err == nil {
				out[key] = p
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RecordDownloads adds items to the history.
func RecordDownloads(records []DownloadRecord) error {
	if len(records) == 0 {
		return nil
	}
	if err := ensureDB(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO download_history
		(media_key, tweet_id, username, media_type, url, path, downloaded_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := time.Now()
	for _, r := range records {
		if _, err := stmt.Exec(r.Key, r.TweetID, r.Username, r.MediaType, r.URL, r.Path, now); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DownloadHistoryCount returns how many items the history holds.
func DownloadHistoryCount() (int, error) {
	if err := ensureDB(); err != nil {
		return 0, err
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM download_history`).Scan(&n)
	return n, err
}

// ClearDownloadHistory forgets every download, so the next jobs check the
// disk alone again.
func ClearDownloadHistory() (int64, error) {
	if err := ensureDB(); err != nil {
		return 0, err
	}
	res, err := db.Exec(`DELETE FROM download_history`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
