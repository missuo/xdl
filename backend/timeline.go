package backend

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// What XDL keeps of a timeline: the account and one entry per photo, video,
// GIF or text post, as the library stores them (the JSON field names are
// the library's format, kept as they are so older libraries still read).

// TweetIDString is a post ID, written to JSON as a string (IDs overflow a
// JSON number's exact range) and read from a string or a number.
type TweetIDString int64

func (t TweetIDString) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(t), 10) + `"`), nil
}

func (t *TweetIDString) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "" || s == "null" {
		*t = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("a post ID must be a number: %s", data)
	}
	*t = TweetIDString(n)
	return nil
}

// TimelineEntry is one item of a timeline.
type TimelineEntry struct {
	URL              string        `json:"url"`
	Date             string        `json:"date"`
	TweetID          TweetIDString `json:"tweet_id"`
	Type             string        `json:"type"` // photo, video, animated_gif, text
	IsRetweet        bool          `json:"is_retweet"`
	Extension        string        `json:"extension"`
	Width            int           `json:"width"`
	Height           int           `json:"height"`
	Thumbnail        string        `json:"thumbnail,omitempty"`
	DurationMS       int           `json:"duration_ms,omitempty"`
	AltText          string        `json:"alt_text,omitempty"`
	Content          string        `json:"content,omitempty"`
	ViewCount        int           `json:"view_count,omitempty"`
	BookmarkCount    int           `json:"bookmark_count,omitempty"`
	FavoriteCount    int           `json:"favorite_count,omitempty"`
	RetweetCount     int           `json:"retweet_count,omitempty"`
	ReplyCount       int           `json:"reply_count,omitempty"`
	QuoteCount       int           `json:"quote_count,omitempty"`
	Source           string        `json:"source,omitempty"`
	Verified         bool          `json:"verified,omitempty"`
	OriginalFilename string        `json:"original_filename,omitempty"`
	AuthorUsername   string        `json:"author_username,omitempty"`
	AuthorName       string        `json:"author_name,omitempty"`
}

// ImageVariant is one size of a profile picture or banner.
type ImageVariant struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// AccountInfo is the account a timeline belongs to.
type AccountInfo struct {
	Name           string         `json:"name"` // the handle
	Nick           string         `json:"nick"` // the display name
	Date           string         `json:"date"` // when it joined
	FollowersCount int            `json:"followers_count"`
	FriendsCount   int            `json:"friends_count"`
	ProfileImage   string         `json:"profile_image"`
	StatusesCount  int            `json:"statuses_count"`
	Bio            string         `json:"bio,omitempty"`
	Banner         string         `json:"banner,omitempty"`
	Location       string         `json:"location,omitempty"`
	URL            string         `json:"url,omitempty"`
	MediaCount     int            `json:"media_count,omitempty"`
	Verified       bool           `json:"verified,omitempty"`
	BlueVerified   bool           `json:"blue_verified,omitempty"`
	Protected      bool           `json:"protected,omitempty"`
	AvatarVariants []ImageVariant `json:"avatar_variants,omitempty"`
	BannerVariants []ImageVariant `json:"banner_variants,omitempty"`
}

// ExtractMetadata describes a page of a fetch.
type ExtractMetadata struct {
	NewEntries int    `json:"new_entries"`
	Page       int    `json:"page"`
	BatchSize  int    `json:"batch_size"`
	HasMore    bool   `json:"has_more"`
	Cursor     string `json:"cursor,omitempty"`
	Completed  bool   `json:"completed,omitempty"`
}

// TwitterResponse is one page of a timeline.
type TwitterResponse struct {
	AccountInfo AccountInfo     `json:"account_info"`
	TotalURLs   int             `json:"total_urls"`
	Timeline    []TimelineEntry `json:"timeline"`
	Metadata    ExtractMetadata `json:"metadata"`
	Cursor      string          `json:"cursor,omitempty"`
	Completed   bool            `json:"completed,omitempty"`
}

// TimelineRequest asks for one page of a timeline.
type TimelineRequest struct {
	Username      string `json:"username"`
	AuthToken     string `json:"auth_token"`
	TimelineType  string `json:"timeline_type"` // media, timeline, tweets, with_replies, likes, bookmarks
	BatchSize     int    `json:"batch_size"`
	Page          int    `json:"page"`
	MediaType     string `json:"media_type"`
	Retweets      bool   `json:"retweets"`
	Cursor        string `json:"cursor,omitempty"`
	IncludePhotos bool   `json:"include_photos"`
	IncludeVideos bool   `json:"include_videos"`
	IncludeGifs   bool   `json:"include_gifs"`
	IncludeText   bool   `json:"include_text"`
	VideoQuality  string `json:"video_quality,omitempty"` // highest, lowest, or a height as "720"
	ImageSize     string `json:"image_size,omitempty"`    // orig, large, medium, small
	Proxy         string `json:"-"`                       // the proxy setting, "" for none
}

// ExtractTimeline fetches one page of a timeline from X.
func ExtractTimeline(ctx context.Context, req TimelineRequest) (*TwitterResponse, error) {
	session, err := getXSession(req.AuthToken, req.Proxy)
	if err != nil {
		return nil, err
	}
	kind := req.TimelineType
	if kind == "" {
		kind = "media"
		if req.IncludeText {
			kind = "tweets"
		}
	}
	pg, err := fetchTimeline(ctx, session, req, kind)
	if err != nil {
		return nil, err
	}

	info := pg.account.info(cleanUsername(req.Username))
	switch kind {
	case "bookmarks":
		info.Name, info.Nick = "bookmarks", "My Bookmarks"
	case "likes":
		info.Name, info.Nick = "likes", "My Likes"
	}
	entries := timelineEntries(pg.posts, entryOptions{
		photos: req.IncludePhotos, videos: req.IncludeVideos, gifs: req.IncludeGifs, text: req.IncludeText,
		videoQuality: req.VideoQuality, imageSize: req.ImageSize,
	})
	return &TwitterResponse{
		AccountInfo: info,
		TotalURLs:   len(entries),
		Timeline:    entries,
		Metadata: ExtractMetadata{
			NewEntries: len(entries),
			Page:       req.Page,
			BatchSize:  req.BatchSize,
			HasMore:    pg.more,
			Cursor:     pg.next,
			Completed:  !pg.more,
		},
		Cursor:    pg.next,
		Completed: !pg.more,
	}, nil
}

// The posts of a page as XDL reads them from X, before they become entries.

type person struct {
	ID, Handle, Name, Avatar string
	Verified                 bool // by X, or by subscription
}

// clip is one encoding of a video.
type clip struct {
	ContentType string
	URL         string
	Bitrate     int
	Resolution  string // "1280x720", from the URL
}

type attachment struct {
	Kind          string // photo, video, animated_gif
	ID, Key       string
	Image         string // the picture, or a video's poster
	Width, Height int
	DurationMS    int
	AltText       string
	Clips         []clip
}

type post struct {
	ID, Text, Lang, CreatedAt, Source string
	Author                            person
	RepostedBy                        *person // set for a repost, whose fields are the original's
	Reply, Quote, Sensitive           bool
	Likes, Reposts, Replies, Quotes   int
	Bookmarks, Views                  int
	Hashtags                          []string
	Media                             []attachment
}

type profile struct {
	ID, Handle, Name, Bio     string
	Avatar, Banner            string
	Location, URL, CreatedAt  string
	Followers, Following      int
	Posts, MediaCount         int
	Protected, Verified, Blue bool
	AvatarSizes, BannerSizes  []ImageVariant
}

type timelinePage struct {
	account profile
	posts   []post
	next    string
	more    bool
}

func (p profile) info(fallback string) AccountInfo {
	return AccountInfo{
		Name:           firstNonEmpty(p.Handle, fallback),
		Nick:           firstNonEmpty(p.Name, fallback),
		Date:           p.CreatedAt,
		FollowersCount: p.Followers,
		FriendsCount:   p.Following,
		ProfileImage:   p.Avatar,
		StatusesCount:  p.Posts,
		Bio:            p.Bio,
		Banner:         p.Banner,
		Location:       p.Location,
		URL:            p.URL,
		MediaCount:     p.MediaCount,
		Verified:       p.Verified,
		BlueVerified:   p.Blue,
		Protected:      p.Protected,
		AvatarVariants: p.AvatarSizes,
		BannerVariants: p.BannerSizes,
	}
}

type entryOptions struct {
	photos, videos, gifs, text bool
	videoQuality, imageSize    string
}

func (o entryOptions) wants(kind string) bool {
	switch kind {
	case "photo":
		return o.photos
	case "video":
		return o.videos
	case "animated_gif", "gif":
		return o.gifs
	}
	return true
}

// timelineEntries makes one entry per attachment the options want, and one
// for a post without any when they want text.
func timelineEntries(posts []post, o entryOptions) []TimelineEntry {
	var out []TimelineEntry
	for _, p := range posts {
		base := TimelineEntry{
			Date:           p.CreatedAt,
			TweetID:        parsePostID(p.ID),
			IsRetweet:      p.RepostedBy != nil,
			Content:        p.Text,
			ViewCount:      p.Views,
			BookmarkCount:  p.Bookmarks,
			FavoriteCount:  p.Likes,
			RetweetCount:   p.Reposts,
			ReplyCount:     p.Replies,
			QuoteCount:     p.Quotes,
			Source:         p.Source,
			Verified:       p.Author.Verified,
			AuthorUsername: p.Author.Handle,
			AuthorName:     p.Author.Name,
		}
		if len(p.Media) == 0 {
			if o.text && strings.TrimSpace(p.Text) != "" {
				e := base
				e.Type, e.Extension = "text", "txt"
				out = append(out, e)
			}
			continue
		}
		for _, m := range p.Media {
			if !o.wants(m.Kind) {
				continue
			}
			e := base
			e.Type = m.Kind
			e.Width, e.Height = m.Width, m.Height
			e.DurationMS, e.AltText = m.DurationMS, m.AltText
			e.Thumbnail = sizedImage(m.Image, "360x360")
			if m.Kind == "photo" {
				e.URL = sizedImage(m.Image, firstNonEmpty(o.imageSize, "orig"))
				e.Extension = imageFormat(e.URL)
			} else {
				e.URL = pickClip(m, o.videoQuality)
				e.Extension = "mp4"
			}
			out = append(out, e)
		}
	}
	return out
}

func parsePostID(s string) TweetIDString {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return TweetIDString(n)
}

// sizedImage names a size of a pbs.twimg.com picture:
// .../media/ID.jpg becomes .../media/ID?format=jpg&name=<size>.
func sizedImage(raw, size string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	path, format := u.Path, "jpg"
	if dot := strings.LastIndexByte(path, '.'); dot > strings.LastIndexByte(path, '/') {
		path, format = path[:dot], path[dot+1:]
	}
	return u.Scheme + "://" + u.Host + path + "?format=" + format + "&name=" + size
}

func imageFormat(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		if f := u.Query().Get("format"); f != "" {
			return f
		}
	}
	return "jpg"
}

// pickClip chooses a video's MP4 by quality: the highest bitrate, the
// lowest, or the one nearest a height such as "720".
func pickClip(m attachment, quality string) string {
	var mp4 []clip
	for _, c := range m.Clips {
		if c.ContentType == "video/mp4" && c.URL != "" {
			mp4 = append(mp4, c)
		}
	}
	if len(mp4) == 0 {
		if len(m.Clips) > 0 {
			return m.Clips[len(m.Clips)-1].URL
		}
		return m.Image
	}
	slices.SortStableFunc(mp4, func(a, b clip) int { return a.Bitrate - b.Bitrate })
	switch quality {
	case "", "highest":
		return mp4[len(mp4)-1].URL
	case "lowest":
		return mp4[0].URL
	}
	want, err := strconv.Atoi(quality)
	if err != nil || want <= 0 {
		return mp4[len(mp4)-1].URL
	}
	best, gap := mp4[0], -1
	for _, c := range mp4 {
		d := clipHeight(c) - want
		if d < 0 {
			d = -d
		}
		if gap < 0 || d < gap {
			best, gap = c, d
		}
	}
	return best.URL
}

func clipHeight(c clip) int {
	if _, h, ok := strings.Cut(c.Resolution, "x"); ok {
		n, _ := strconv.Atoi(h)
		return n
	}
	return 0
}

// cleanUsername turns "@user", "x.com/user" or a profile URL into the
// handle; it leaves other X paths (/i/…, /search…) as they are.
func cleanUsername(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "@")
	if !strings.Contains(s, "x.com/") && !strings.Contains(s, "twitter.com/") {
		return s
	}
	raw := s
	if !strings.Contains(raw, "://") {
		raw = "https://" + strings.TrimPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return s
	}
	first, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
	switch strings.ToLower(first) {
	case "", "i", "search", "home", "explore", "settings", "messages", "notifications":
		return s
	}
	return first
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
