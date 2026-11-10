package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type TweetIDString int64

func (t TweetIDString) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%d"`, t)), nil
}

func (t *TweetIDString) UnmarshalJSON(data []byte) error {

	var num int64
	if err := json.Unmarshal(data, &num); err == nil {
		*t = TweetIDString(num)
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		parsed, err := fmt.Sscanf(str, "%d", &num)
		if err != nil || parsed != 1 {
			return fmt.Errorf("invalid tweet_id string: %s", str)
		}
		*t = TweetIDString(num)
		return nil
	}
	return fmt.Errorf("tweet_id must be number or string")
}

// ImageVariant is one size of a profile picture or banner.
type ImageVariant = richThumb

type richThumb struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type richSize struct {
	W      int    `json:"w"`
	H      int    `json:"h"`
	Resize string `json:"resize"`
	URL    string `json:"url"`
}

type richVariant struct {
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Bitrate     int    `json:"bitrate"`
	Resolution  string `json:"resolution"`
}

type richMedia struct {
	Type        string              `json:"type"`
	MediaID     string              `json:"media_id"`
	MediaKey    string              `json:"media_key"`
	Thumbnail   string              `json:"thumbnail"`
	ExpandedURL string              `json:"expanded_url"`
	Width       int                 `json:"width"`
	Height      int                 `json:"height"`
	AspectRatio []int               `json:"aspect_ratio"`
	DurationMS  int                 `json:"duration_ms"`
	AltText     string              `json:"alt_text"`
	Sizes       map[string]richSize `json:"sizes"`
	Thumbs      []richThumb         `json:"thumbs"`
	Variants    []richVariant       `json:"variants"`
}

type richMiniUser struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	Verified     bool   `json:"verified"`
	BlueVerified bool   `json:"blue_verified"`
}

type richStats struct {
	Likes     int `json:"likes"`
	Retweets  int `json:"retweets"`
	Replies   int `json:"replies"`
	Quotes    int `json:"quotes"`
	Bookmarks int `json:"bookmarks"`
	Views     int `json:"views"`
}

type richTweet struct {
	TweetID    string        `json:"tweet_id"`
	URL        string        `json:"url"`
	Text       string        `json:"text"`
	Lang       string        `json:"lang"`
	CreatedAt  string        `json:"created_at"`
	Source     string        `json:"source"`
	Sensitive  bool          `json:"sensitive"`
	IsRepost   bool          `json:"is_repost"`
	IsReply    bool          `json:"is_reply"`
	IsQuote    bool          `json:"is_quote"`
	RepostedBy *richMiniUser `json:"reposted_by"`
	Author     richMiniUser  `json:"author"`
	Stats      richStats     `json:"stats"`
	Media      []richMedia   `json:"media"`
	Hashtags   []string      `json:"hashtags"`
}

type richAccount struct {
	ID             string      `json:"id"`
	Username       string      `json:"username"`
	Name           string      `json:"name"`
	Bio            string      `json:"bio"`
	Avatar         string      `json:"avatar"`
	AvatarVariants []richThumb `json:"avatar_variants"`
	Banner         string      `json:"banner"`
	BannerVariants []richThumb `json:"banner_variants"`
	Location       string      `json:"location"`
	URL            string      `json:"url"`
	CreatedAt      string      `json:"created_at"`
	Followers      int         `json:"followers"`
	Following      int         `json:"following"`
	Tweets         int         `json:"tweets"`
	MediaCount     int         `json:"media_count"`
	Protected      bool        `json:"protected"`
	Verified       bool        `json:"verified"`
	BlueVerified   bool        `json:"blue_verified"`
}

type richResponse struct {
	Account    richAccount `json:"account"`
	Tweets     []richTweet `json:"tweets"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

type TimelineEntry struct {
	URL              string        `json:"url"`
	Date             string        `json:"date"`
	TweetID          TweetIDString `json:"tweet_id"`
	Type             string        `json:"type"`
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

type AccountInfo struct {
	Name           string      `json:"name"`
	Nick           string      `json:"nick"`
	Date           string      `json:"date"`
	FollowersCount int         `json:"followers_count"`
	FriendsCount   int         `json:"friends_count"`
	ProfileImage   string      `json:"profile_image"`
	StatusesCount  int         `json:"statuses_count"`
	Bio            string      `json:"bio,omitempty"`
	Banner         string      `json:"banner,omitempty"`
	Location       string      `json:"location,omitempty"`
	URL            string      `json:"url,omitempty"`
	MediaCount     int         `json:"media_count,omitempty"`
	Verified       bool        `json:"verified,omitempty"`
	BlueVerified   bool        `json:"blue_verified,omitempty"`
	Protected      bool        `json:"protected,omitempty"`
	AvatarVariants []richThumb `json:"avatar_variants,omitempty"`
	BannerVariants []richThumb `json:"banner_variants,omitempty"`
}

type ExtractMetadata struct {
	NewEntries int    `json:"new_entries"`
	Page       int    `json:"page"`
	BatchSize  int    `json:"batch_size"`
	HasMore    bool   `json:"has_more"`
	Cursor     string `json:"cursor,omitempty"`
	Completed  bool   `json:"completed,omitempty"`
}

type TwitterResponse struct {
	AccountInfo AccountInfo     `json:"account_info"`
	TotalURLs   int             `json:"total_urls"`
	Timeline    []TimelineEntry `json:"timeline"`
	Metadata    ExtractMetadata `json:"metadata"`
	Cursor      string          `json:"cursor,omitempty"`
	Completed   bool            `json:"completed,omitempty"`
}

type TimelineRequest struct {
	Username      string `json:"username"`
	AuthToken     string `json:"auth_token"`
	TimelineType  string `json:"timeline_type"`
	BatchSize     int    `json:"batch_size"`
	Page          int    `json:"page"`
	MediaType     string `json:"media_type"`
	Retweets      bool   `json:"retweets"`
	Cursor        string `json:"cursor,omitempty"`
	IncludePhotos bool   `json:"include_photos"`
	IncludeVideos bool   `json:"include_videos"`
	IncludeGifs   bool   `json:"include_gifs"`
	IncludeText   bool   `json:"include_text"`
	VideoQuality  string `json:"video_quality,omitempty"`
	ImageSize     string `json:"image_size,omitempty"`
	Proxy         string `json:"-"` // the proxy setting, "" for none
}

func cleanUsername(username string) string {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")

	if strings.Contains(username, "x.com/") || strings.Contains(username, "twitter.com/") {
		parsed := username
		if !strings.HasPrefix(parsed, "http://") && !strings.HasPrefix(parsed, "https://") {
			parsed = "https://" + strings.TrimPrefix(parsed, "//")
		}
		if u, err := url.Parse(parsed); err == nil {
			segments := strings.Split(strings.Trim(u.Path, "/"), "/")

			if len(segments) > 0 && segments[0] != "" {
				firstSegment := strings.ToLower(segments[0])

				if firstSegment == "i" || firstSegment == "search" || firstSegment == "home" || firstSegment == "explore" || firstSegment == "settings" || firstSegment == "messages" || firstSegment == "notifications" {
					return username
				}
				return segments[0]
			}
		}
	}

	return username
}

func parseTweetID(s string) TweetIDString {
	var n int64
	_, _ = fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return TweetIDString(n)
}

func bestPhotoURL(m richMedia, size string) string {
	if size == "" {
		size = "orig"
	}
	if size == "orig" {
		for _, t := range m.Thumbs {
			if t.Name == "orig" {
				return t.URL
			}
		}
	} else if s, ok := m.Sizes[size]; ok && s.URL != "" {
		return s.URL
	}
	if s, ok := m.Sizes["large"]; ok && s.URL != "" {
		return s.URL
	}
	if s, ok := m.Sizes["medium"]; ok && s.URL != "" {
		return s.URL
	}
	if s, ok := m.Sizes["small"]; ok && s.URL != "" {
		return s.URL
	}
	return m.Thumbnail
}

func bestVideoURL(m richMedia, quality string) string {
	mp4s := make([]richVariant, 0, len(m.Variants))
	for _, v := range m.Variants {
		if v.ContentType == "video/mp4" && v.URL != "" {
			mp4s = append(mp4s, v)
		}
	}
	if len(mp4s) == 0 {
		if len(m.Variants) > 0 {
			return m.Variants[len(m.Variants)-1].URL
		}
		return m.Thumbnail
	}
	sort.Slice(mp4s, func(i, j int) bool { return mp4s[i].Bitrate < mp4s[j].Bitrate })

	switch quality {
	case "lowest":
		return mp4s[0].URL
	case "", "highest":
		return mp4s[len(mp4s)-1].URL
	}

	target := 0
	_, _ = fmt.Sscanf(quality, "%d", &target)
	if target == 0 {
		return mp4s[len(mp4s)-1].URL
	}
	best := mp4s[0]
	bestDelta := 1 << 30
	for _, v := range mp4s {
		h := resolutionHeight(v.Resolution)
		delta := h - target
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			bestDelta = delta
			best = v
		}
	}
	return best.URL
}

func resolutionHeight(resolution string) int {
	if i := strings.Index(resolution, "x"); i >= 0 {
		h := 0
		_, _ = fmt.Sscanf(resolution[i+1:], "%d", &h)
		return h
	}
	return 0
}

func previewURL(m richMedia) string {
	for _, t := range m.Thumbs {
		if t.Name == "360x360" {
			return t.URL
		}
	}
	if s, ok := m.Sizes["small"]; ok && s.URL != "" {
		return s.URL
	}
	return m.Thumbnail
}

func extensionFor(downloadURL, mediaType string) string {
	switch mediaType {
	case "video", "gif", "animated_gif":
		return "mp4"
	}
	if i := strings.Index(downloadURL, "format="); i >= 0 {
		rest := downloadURL[i+len("format="):]
		if amp := strings.IndexByte(rest, '&'); amp >= 0 {
			rest = rest[:amp]
		}
		if rest != "" {
			return rest
		}
	}
	return "jpg"
}

func isVerified(u richMiniUser) bool {
	return u.Verified || u.BlueVerified
}

func textEntry(t richTweet) TimelineEntry {
	return TimelineEntry{
		URL:            "",
		Date:           t.CreatedAt,
		TweetID:        parseTweetID(t.TweetID),
		Type:           "text",
		IsRetweet:      t.IsRepost,
		Extension:      "txt",
		Content:        t.Text,
		ViewCount:      t.Stats.Views,
		BookmarkCount:  t.Stats.Bookmarks,
		FavoriteCount:  t.Stats.Likes,
		RetweetCount:   t.Stats.Retweets,
		ReplyCount:     t.Stats.Replies,
		QuoteCount:     t.Stats.Quotes,
		Source:         t.Source,
		Verified:       isVerified(t.Author),
		AuthorUsername: t.Author.Username,
		AuthorName:     t.Author.Name,
	}
}

func mediaEntry(t richTweet, m richMedia, videoQuality, imageSize string) TimelineEntry {
	var downloadURL string
	switch m.Type {
	case "video", "animated_gif", "gif":
		downloadURL = bestVideoURL(m, videoQuality)
	default:
		downloadURL = bestPhotoURL(m, imageSize)
	}
	return TimelineEntry{
		URL:            downloadURL,
		Date:           t.CreatedAt,
		TweetID:        parseTweetID(t.TweetID),
		Type:           m.Type,
		IsRetweet:      t.IsRepost,
		Extension:      extensionFor(downloadURL, m.Type),
		Width:          m.Width,
		Height:         m.Height,
		Thumbnail:      previewURL(m),
		DurationMS:     m.DurationMS,
		AltText:        m.AltText,
		Content:        t.Text,
		ViewCount:      t.Stats.Views,
		BookmarkCount:  t.Stats.Bookmarks,
		FavoriteCount:  t.Stats.Likes,
		RetweetCount:   t.Stats.Retweets,
		ReplyCount:     t.Stats.Replies,
		QuoteCount:     t.Stats.Quotes,
		Source:         t.Source,
		Verified:       isVerified(t.Author),
		AuthorUsername: t.Author.Username,
		AuthorName:     t.Author.Name,
	}
}

type FlattenOptions struct {
	IncludePhotos bool
	IncludeVideos bool
	IncludeGifs   bool
	IncludeText   bool
	VideoQuality  string
	ImageSize     string
}

func flattenTweets(tweets []richTweet, opts FlattenOptions) []TimelineEntry {
	out := make([]TimelineEntry, 0, len(tweets))
	for _, t := range tweets {
		if len(t.Media) == 0 {
			if opts.IncludeText && strings.TrimSpace(t.Text) != "" {
				out = append(out, textEntry(t))
			}
			continue
		}
		for _, m := range t.Media {
			switch m.Type {
			case "photo":
				if !opts.IncludePhotos {
					continue
				}
			case "video":
				if !opts.IncludeVideos {
					continue
				}
			case "animated_gif", "gif":
				if !opts.IncludeGifs {
					continue
				}
			}
			out = append(out, mediaEntry(t, m, opts.VideoQuality, opts.ImageSize))
		}
	}
	return out
}

func buildAccountInfo(acc richAccount, fallbackUsername string) AccountInfo {
	info := AccountInfo{
		Name: fallbackUsername,
		Nick: fallbackUsername,
	}
	if acc.Username != "" {
		info.Name = acc.Username
	}
	if acc.Name != "" {
		info.Nick = acc.Name
	}
	info.Date = acc.CreatedAt
	info.FollowersCount = acc.Followers
	info.FriendsCount = acc.Following
	info.ProfileImage = acc.Avatar
	info.StatusesCount = acc.Tweets
	info.Bio = acc.Bio
	info.Banner = acc.Banner
	info.Location = acc.Location
	info.URL = acc.URL
	info.MediaCount = acc.MediaCount
	info.Verified = acc.Verified
	info.BlueVerified = acc.BlueVerified
	info.Protected = acc.Protected
	info.AvatarVariants = acc.AvatarVariants
	info.BannerVariants = acc.BannerVariants
	return info
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
	resp, err := fetchTimeline(ctx, session, req, kind)
	if err != nil {
		return nil, err
	}

	accountInfo := buildAccountInfo(resp.Account, cleanUsername(req.Username))
	switch kind {
	case "bookmarks":
		accountInfo.Name, accountInfo.Nick = "bookmarks", "My Bookmarks"
	case "likes":
		accountInfo.Name, accountInfo.Nick = "likes", "My Likes"
	}
	timeline := flattenTweets(resp.Tweets, FlattenOptions{
		IncludePhotos: req.IncludePhotos,
		IncludeVideos: req.IncludeVideos,
		IncludeGifs:   req.IncludeGifs,
		IncludeText:   req.IncludeText,
		VideoQuality:  req.VideoQuality,
		ImageSize:     req.ImageSize,
	})
	completed := !resp.HasMore
	return &TwitterResponse{
		AccountInfo: accountInfo,
		TotalURLs:   len(timeline),
		Timeline:    timeline,
		Metadata: ExtractMetadata{
			NewEntries: len(timeline),
			Page:       req.Page,
			BatchSize:  req.BatchSize,
			HasMore:    resp.HasMore,
			Cursor:     resp.NextCursor,
			Completed:  completed,
		},
		Cursor:    resp.NextCursor,
		Completed: completed,
	}, nil
}
