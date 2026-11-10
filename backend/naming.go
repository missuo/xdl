package backend

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Where a downloaded item goes:
//
//	<download folder>/<folder template>/<images|videos|gifs|texts>/<file template>.<ext>
//
// The folder template knows {handle} (also {username}), {name} (also
// {account_name}) and {date}, the day of the download; the file template
// those but {date}, the post's time (UTC, 20060102_150405), and {tweet_id},
// {index} (the item's place in its post, 01…), {media_id} (X's name of the
// file) and {type}. Items of a download that end up with one name are
// told apart by _01, _02…

const (
	defaultFileTemplate   = "{handle}_{date}_{tweet_id}"
	defaultFolderTemplate = "{handle}"
)

// kindFolder is the folder of a kind of item.
func kindFolder(kind string) string {
	switch kind {
	case "photo":
		return "images"
	case "video":
		return "videos"
	case "animated_gif", "gif":
		return "gifs"
	case "text":
		return "texts"
	}
	return "other"
}

func fillTemplate(template string, fields map[string]string) string {
	var b strings.Builder
	for {
		open := strings.IndexByte(template, '{')
		if open < 0 {
			b.WriteString(template)
			break
		}
		end := strings.IndexByte(template[open:], '}')
		if end < 0 {
			b.WriteString(template)
			break
		}
		b.WriteString(template[:open])
		token := template[open+1 : open+end]
		if v, ok := fields[token]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(template[open : open+end+1])
		}
		template = template[open+end+1:]
	}
	return b.String()
}

func folderName(template, handle, name string, day time.Time) string {
	if strings.TrimSpace(template) == "" {
		template = defaultFolderTemplate
	}
	out := sanitizeFilenamePart(fillTemplate(template, map[string]string{
		"handle": sanitizeFilenamePart(handle), "username": sanitizeFilenamePart(handle),
		"name": sanitizeFilenamePart(name), "account_name": sanitizeFilenamePart(name),
		"date": day.Format("20060102"),
	}))
	return firstNonEmpty(out, sanitizeFilenamePart(handle), "x")
}

func fileName(template string, it MediaItem, handle, name string, index int) string {
	if strings.TrimSpace(template) == "" {
		template = defaultFileTemplate
	}
	when := postTime(it.Date)
	mediaID := it.OriginalFilename
	if mediaID == "" {
		mediaID = ExtractOriginalFilename(it.URL)
	}
	out := sanitizeFilenamePart(fillTemplate(template, map[string]string{
		"handle": sanitizeFilenamePart(handle), "username": sanitizeFilenamePart(handle),
		"name": sanitizeFilenamePart(name), "account_name": sanitizeFilenamePart(name),
		"date":     when,
		"tweet_id": strconv.FormatInt(it.TweetID, 10),
		"index":    twoDigits(index),
		"media_id": sanitizeFilenamePart(mediaID),
		"type":     sanitizeFilenamePart(it.Type),
	}))
	if out == "" {
		out = sanitizeFilenamePart(handle + "_" + when + "_" + strconv.FormatInt(it.TweetID, 10))
	}
	return out
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// postTime writes a post's time as 20060102_150405, in the time zone of the
// date (UTC for X's), or zeros when the date does not read.
func postTime(date string) string {
	for _, layout := range []string{time.RubyDate, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, strings.TrimSpace(date)); err == nil {
			return t.Format("20060102_150405")
		}
	}
	return "00000000_000000"
}

// fileExtension is the extension a downloaded item gets: X's format= for
// pictures, else the URL's, else the kind's.
func fileExtension(rawURL, kind string) string {
	switch kind {
	case "text":
		return ".txt"
	case "video", "animated_gif", "gif":
		return ".mp4"
	}
	if u, err := url.Parse(rawURL); err == nil {
		if f := u.Query().Get("format"); f != "" {
			return "." + strings.ToLower(f)
		}
		if ext := path.Ext(u.Path); ext != "" {
			return strings.ToLower(ext)
		}
	}
	return ".jpg"
}

var reFileStem = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ExtractOriginalFilename is X's name of a media file, without its
// extension: the ID of a pbs.twimg.com picture or of a GIF, the last part
// of a video's path. "" when the URL has none.
func ExtractOriginalFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	stem := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	if !reFileStem.MatchString(stem) {
		return ""
	}
	switch {
	case strings.HasSuffix(u.Host, "pbs.twimg.com") && strings.Contains(u.Path, "/media/"):
		return stem
	case strings.HasSuffix(u.Host, "video.twimg.com") && strings.Contains(u.Path, "/tweet_video/"):
		return stem
	case strings.HasSuffix(u.Host, "video.twimg.com"):
		// A video's file is named by its encoding, as "AbCdEfGh12345678":
		// a short or all-digit name says nothing.
		if len(stem) >= 8 && strings.ContainsAny(strings.ToLower(stem), "abcdefghijklmnopqrstuvwxyz") {
			return stem
		}
	}
	return ""
}

// sanitizeFilenamePart makes s a valid file name on every system: without
// the characters Windows forbids or control characters, without trailing
// dots or spaces, and not one of Windows' device names.
func sanitizeFilenamePart(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`\/:*?"<>|`, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(strings.TrimSpace(s), ". ")
	if reserved(s) {
		s += "_"
	}
	return s
}

func reserved(name string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}
