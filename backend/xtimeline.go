package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The parts of X's GraphQL responses XDL reads. Accounts come in two
// shapes, the older one with everything under legacy, so both are read.

type gqlUser struct {
	Typename       string `json:"__typename"`
	RestID         string `json:"rest_id"`
	IsBlueVerified bool   `json:"is_blue_verified"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Core           struct {
		Name       string `json:"name"`
		ScreenName string `json:"screen_name"`
		CreatedAt  string `json:"created_at"`
	} `json:"core"`
	Avatar struct {
		ImageURL string `json:"image_url"`
	} `json:"avatar"`
	Location struct {
		Location string `json:"location"`
	} `json:"location"`
	Privacy struct {
		Protected bool `json:"protected"`
	} `json:"privacy"`
	Verification struct {
		Verified bool `json:"verified"`
	} `json:"verification"`
	// The 2026 shape moves counts and the profile out of legacy.
	RelationshipCounts struct {
		Followers int `json:"followers"`
		Following int `json:"following"`
	} `json:"relationship_counts"`
	TweetCounts struct {
		Tweets      int `json:"tweets"`
		MediaTweets int `json:"media_tweets"`
	} `json:"tweet_counts"`
	ProfileBio struct {
		Description string `json:"description"`
	} `json:"profile_bio"`
	Banner struct {
		ImageURL string `json:"image_url"`
	} `json:"banner"`
	Website struct {
		URL string `json:"url"`
	} `json:"website"`
	Legacy struct {
		Name                 string `json:"name"`
		ScreenName           string `json:"screen_name"`
		Description          string `json:"description"`
		Location             string `json:"location"`
		URL                  string `json:"url"`
		CreatedAt            string `json:"created_at"`
		ProfileImageURLHTTPS string `json:"profile_image_url_https"`
		ProfileBannerURL     string `json:"profile_banner_url"`
		FollowersCount       int    `json:"followers_count"`
		FriendsCount         int    `json:"friends_count"`
		StatusesCount        int    `json:"statuses_count"`
		MediaCount           int    `json:"media_count"`
		Protected            bool   `json:"protected"`
		Verified             bool   `json:"verified"`
		Entities             struct {
			URL struct {
				URLs []struct {
					ExpandedURL string `json:"expanded_url"`
				} `json:"urls"`
			} `json:"url"`
		} `json:"entities"`
	} `json:"legacy"`
}

func (u *gqlUser) screenName() string { return firstNonEmpty(u.Core.ScreenName, u.Legacy.ScreenName) }
func (u *gqlUser) name() string       { return firstNonEmpty(u.Core.Name, u.Legacy.Name) }
func (u *gqlUser) avatar() string {
	return firstNonEmpty(u.Avatar.ImageURL, u.Legacy.ProfileImageURLHTTPS)
}

type gqlMedia struct {
	IDStr         string `json:"id_str"`
	MediaKey      string `json:"media_key"`
	Type          string `json:"type"`
	MediaURLHTTPS string `json:"media_url_https"`
	ExpandedURL   string `json:"expanded_url"`
	ExtAltText    string `json:"ext_alt_text"`
	OriginalInfo  struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"original_info"`
	Sizes map[string]struct {
		W      int    `json:"w"`
		H      int    `json:"h"`
		Resize string `json:"resize"`
	} `json:"sizes"`
	VideoInfo struct {
		AspectRatio    []int `json:"aspect_ratio"`
		DurationMillis int   `json:"duration_millis"`
		Variants       []struct {
			ContentType string `json:"content_type"`
			URL         string `json:"url"`
			Bitrate     int    `json:"bitrate"`
		} `json:"variants"`
	} `json:"video_info"`
}

type gqlTweet struct {
	Typename string    `json:"__typename"`
	RestID   string    `json:"rest_id"`
	Tweet    *gqlTweet `json:"tweet"` // TweetWithVisibilityResults
	Source   string    `json:"source"`
	Core     struct {
		UserResults struct {
			Result *gqlUser `json:"result"`
		} `json:"user_results"`
	} `json:"core"`
	Views struct {
		Count string `json:"count"`
	} `json:"views"`
	NoteTweet struct {
		NoteTweetResults struct {
			Result struct {
				Text string `json:"text"`
			} `json:"result"`
		} `json:"note_tweet_results"`
	} `json:"note_tweet"`
	Legacy *struct {
		IDStr                 string `json:"id_str"`
		FullText              string `json:"full_text"`
		CreatedAt             string `json:"created_at"`
		Lang                  string `json:"lang"`
		PossiblySensitive     bool   `json:"possibly_sensitive"`
		IsQuoteStatus         bool   `json:"is_quote_status"`
		InReplyToStatusIDStr  string `json:"in_reply_to_status_id_str"`
		FavoriteCount         int    `json:"favorite_count"`
		RetweetCount          int    `json:"retweet_count"`
		ReplyCount            int    `json:"reply_count"`
		QuoteCount            int    `json:"quote_count"`
		BookmarkCount         int    `json:"bookmark_count"`
		RetweetedStatusResult struct {
			Result *gqlTweet `json:"result"`
		} `json:"retweeted_status_result"`
		Entities struct {
			Hashtags []struct {
				Text string `json:"text"`
			} `json:"hashtags"`
		} `json:"entities"`
		ExtendedEntities struct {
			Media []gqlMedia `json:"media"`
		} `json:"extended_entities"`
	} `json:"legacy"`
}

func (t *gqlTweet) unwrap() *gqlTweet {
	for t != nil && t.Tweet != nil && t.Legacy == nil {
		t = t.Tweet
	}
	return t
}

type gqlItemContent struct {
	ItemType     string `json:"itemType"`
	TweetResults struct {
		Result json.RawMessage `json:"result"`
	} `json:"tweet_results"`
	PromotedMetadata json.RawMessage `json:"promotedMetadata"`
}

type gqlModuleItem struct {
	EntryID string `json:"entryId"`
	Item    struct {
		ItemContent *gqlItemContent `json:"itemContent"`
	} `json:"item"`
}

type gqlEntry struct {
	EntryID string `json:"entryId"`
	Content struct {
		EntryType   string          `json:"entryType"`
		CursorType  string          `json:"cursorType"`
		Value       string          `json:"value"`
		ItemContent *gqlItemContent `json:"itemContent"`
		Items       []gqlModuleItem `json:"items"`
	} `json:"content"`
}

type gqlInstruction struct {
	Type        string          `json:"type"`
	Entries     []gqlEntry      `json:"entries"`
	Entry       *gqlEntry       `json:"entry"`
	ModuleItems []gqlModuleItem `json:"moduleItems"`
}

type gqlTimeline struct {
	Timeline struct {
		Instructions []gqlInstruction `json:"instructions"`
	} `json:"timeline"`
}

type gqlTimelineResponse struct {
	Data struct {
		User struct {
			Result struct {
				Typename   string      `json:"__typename"`
				Timeline   gqlTimeline `json:"timeline"`
				TimelineV2 gqlTimeline `json:"timeline_v2"`
			} `json:"result"`
		} `json:"user"`
		BookmarkTimelineV2 gqlTimeline `json:"bookmark_timeline_v2"`
	} `json:"data"`
	Errors []gqlError `json:"errors"`
}

func (r *gqlTimelineResponse) instructions() []gqlInstruction {
	for _, t := range []gqlTimeline{r.Data.User.Result.Timeline, r.Data.User.Result.TimelineV2, r.Data.BookmarkTimelineV2} {
		if len(t.Timeline.Instructions) > 0 {
			return t.Timeline.Instructions
		}
	}
	return nil
}

// timelinePage is one page of a timeline: its tweets in order and the
// cursor of the next.
type timelinePage struct {
	tweets []richTweet
	next   string
}

func parseTimelinePage(body []byte) (*timelinePage, error) {
	var resp gqlTimelineResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("X returned invalid JSON: %v", err)
	}
	instructions := resp.instructions()
	if instructions == nil && len(resp.Errors) > 0 {
		return nil, classifyGraphQLErrors(resp.Errors)
	}
	page := &timelinePage{}
	add := func(id string, c *gqlItemContent) {
		if c == nil || len(c.TweetResults.Result) == 0 || len(c.PromotedMetadata) > 0 && string(c.PromotedMetadata) != "null" {
			return
		}
		if strings.HasPrefix(id, "promoted-") {
			return
		}
		var t gqlTweet
		if err := json.Unmarshal(c.TweetResults.Result, &t); err != nil {
			return // a tweet in a shape XDL can't read is skipped, not fatal
		}
		if rt, ok := toRichTweet(&t); ok {
			page.tweets = append(page.tweets, rt)
		}
	}
	entry := func(e *gqlEntry) {
		switch {
		case e.Content.CursorType == "Bottom" || strings.HasPrefix(e.EntryID, "cursor-bottom-"):
			page.next = e.Content.Value
		case e.Content.ItemContent != nil:
			add(e.EntryID, e.Content.ItemContent)
		default:
			for _, it := range e.Content.Items {
				add(it.EntryID, it.Item.ItemContent)
			}
		}
	}
	for _, in := range instructions {
		for i := range in.Entries {
			entry(&in.Entries[i])
		}
		if in.Entry != nil {
			entry(in.Entry)
		}
		for _, it := range in.ModuleItems {
			add(it.EntryID, it.Item.ItemContent)
		}
	}
	return page, nil
}

func classifyGraphQLErrors(errs []gqlError) error {
	msg := errs[0].Message
	lower := strings.ToLower(msg)
	switch {
	case errs[0].Code == 32 || strings.Contains(lower, "could not authenticate"):
		return ErrXAuth
	case errs[0].Code == 88 || strings.Contains(lower, "rate limit"):
		return &XRateLimitError{}
	case strings.Contains(lower, "suspended"):
		return errors.New("the account is suspended")
	case strings.Contains(lower, "locked"):
		return errors.New("X has locked the account whose auth token XDL uses; unlock it on x.com")
	}
	return fmt.Errorf("X: %s", msg)
}

var reHTMLTag = regexp.MustCompile(`<[^>]*>`)

func toRichTweet(raw *gqlTweet) (richTweet, bool) {
	t := raw.unwrap()
	if t == nil || t.Legacy == nil {
		return richTweet{}, false // a tombstone or a withheld tweet
	}
	if rt := t.Legacy.RetweetedStatusResult.Result.unwrap(); rt != nil && rt.Legacy != nil {
		out, ok := toRichTweet(rt)
		if !ok {
			return richTweet{}, false
		}
		by := miniUser(t.Core.UserResults.Result)
		out.IsRepost = true
		out.RepostedBy = &by
		return out, true
	}
	l := t.Legacy
	id := firstNonEmpty(l.IDStr, t.RestID)
	author := miniUser(t.Core.UserResults.Result)
	text := l.FullText
	if note := t.NoteTweet.NoteTweetResults.Result.Text; note != "" {
		text = note
	}
	views, _ := strconv.Atoi(t.Views.Count)
	out := richTweet{
		TweetID:   id,
		URL:       "https://x.com/" + firstNonEmpty(author.Username, "i") + "/status/" + id,
		Text:      text,
		Lang:      l.Lang,
		CreatedAt: l.CreatedAt,
		Source:    html.UnescapeString(reHTMLTag.ReplaceAllString(t.Source, "")),
		Sensitive: l.PossiblySensitive,
		IsReply:   l.InReplyToStatusIDStr != "",
		IsQuote:   l.IsQuoteStatus,
		Author:    author,
		Stats: richStats{
			Likes:     l.FavoriteCount,
			Retweets:  l.RetweetCount,
			Replies:   l.ReplyCount,
			Quotes:    l.QuoteCount,
			Bookmarks: l.BookmarkCount,
			Views:     views,
		},
	}
	for _, h := range l.Entities.Hashtags {
		out.Hashtags = append(out.Hashtags, h.Text)
	}
	for _, m := range l.ExtendedEntities.Media {
		out.Media = append(out.Media, toRichMedia(m))
	}
	return out, true
}

func miniUser(u *gqlUser) richMiniUser {
	if u == nil {
		return richMiniUser{}
	}
	return richMiniUser{
		UserID:       u.RestID,
		Username:     u.screenName(),
		Name:         u.name(),
		Avatar:       u.avatar(),
		Verified:     u.Verification.Verified || u.Legacy.Verified,
		BlueVerified: u.IsBlueVerified,
	}
}

var reVideoResolution = regexp.MustCompile(`/(\d+x\d+)/`)

// photoURL returns a pbs.twimg.com image at a named size:
// .../media/ID.jpg becomes .../media/ID?format=jpg&name=<size>.
func photoURL(mediaURL, size string) string {
	u, err := url.Parse(mediaURL)
	if err != nil || u.Host == "" {
		return mediaURL
	}
	base, ext := u.Path, "jpg"
	if i := strings.LastIndexByte(base, '.'); i > strings.LastIndexByte(base, '/') {
		base, ext = base[:i], base[i+1:]
	}
	return u.Scheme + "://" + u.Host + base + "?format=" + ext + "&name=" + size
}

func toRichMedia(m gqlMedia) richMedia {
	out := richMedia{
		Type:        m.Type,
		MediaID:     m.IDStr,
		MediaKey:    m.MediaKey,
		Thumbnail:   m.MediaURLHTTPS,
		ExpandedURL: m.ExpandedURL,
		Width:       m.OriginalInfo.Width,
		Height:      m.OriginalInfo.Height,
		AspectRatio: m.VideoInfo.AspectRatio,
		DurationMS:  m.VideoInfo.DurationMillis,
		AltText:     m.ExtAltText,
		Sizes:       map[string]richSize{},
	}
	for name, s := range m.Sizes {
		out.Sizes[name] = richSize{W: s.W, H: s.H, Resize: s.Resize, URL: photoURL(m.MediaURLHTTPS, name)}
	}
	for _, name := range []string{"360x360", "900x900", "4096x4096", "orig"} {
		out.Thumbs = append(out.Thumbs, richThumb{Name: name, URL: photoURL(m.MediaURLHTTPS, name)})
	}
	for _, v := range m.VideoInfo.Variants {
		u := v.URL
		if i := strings.Index(u, "?tag="); i >= 0 {
			u = u[:i]
		}
		res := ""
		if r := reVideoResolution.FindStringSubmatch(u); r != nil {
			res = r[1]
		}
		out.Variants = append(out.Variants, richVariant{ContentType: v.ContentType, URL: u, Bitrate: v.Bitrate, Resolution: res})
	}
	return out
}

func toRichAccount(u *gqlUser) richAccount {
	acc := richAccount{
		ID:           u.RestID,
		Username:     u.screenName(),
		Name:         u.name(),
		Bio:          firstNonEmpty(u.ProfileBio.Description, u.Legacy.Description),
		Avatar:       u.avatar(),
		Banner:       firstNonEmpty(u.Banner.ImageURL, u.Legacy.ProfileBannerURL),
		Location:     firstNonEmpty(u.Location.Location, u.Legacy.Location),
		URL:          firstNonEmpty(u.Website.URL, u.Legacy.URL),
		CreatedAt:    firstNonEmpty(u.Core.CreatedAt, u.Legacy.CreatedAt),
		Followers:    max(u.RelationshipCounts.Followers, u.Legacy.FollowersCount),
		Following:    max(u.RelationshipCounts.Following, u.Legacy.FriendsCount),
		Tweets:       max(u.TweetCounts.Tweets, u.Legacy.StatusesCount),
		MediaCount:   max(u.TweetCounts.MediaTweets, u.Legacy.MediaCount),
		Protected:    u.Privacy.Protected || u.Legacy.Protected,
		Verified:     u.Verification.Verified || u.Legacy.Verified,
		BlueVerified: u.IsBlueVerified,
	}
	if urls := u.Legacy.Entities.URL.URLs; u.Website.URL == "" && len(urls) > 0 && urls[0].ExpandedURL != "" {
		acc.URL = urls[0].ExpandedURL
	}
	// Profile pictures come in fixed sizes: name_normal.jpg, name_400x400.jpg...
	if acc.Avatar != "" {
		original := strings.Replace(acc.Avatar, "_normal.", ".", 1)
		acc.Avatar = original
		if i := strings.LastIndexByte(original, '.'); i > strings.LastIndexByte(original, '/') {
			for _, size := range []string{"mini", "normal", "bigger", "200x200", "400x400"} {
				acc.AvatarVariants = append(acc.AvatarVariants, richThumb{Name: size, URL: original[:i] + "_" + size + original[i:]})
			}
		}
		acc.AvatarVariants = append(acc.AvatarVariants, richThumb{Name: "orig", URL: original})
	}
	if acc.Banner != "" {
		for _, size := range []string{"300x100", "600x200", "1080x360", "1500x500"} {
			acc.BannerVariants = append(acc.BannerVariants, richThumb{Name: size, URL: acc.Banner + "/" + size})
		}
	}
	return acc
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// lookupUser resolves a handle, cached for xUserTTL.
func (s *xSession) lookupUser(ctx context.Context, handle string) (*gqlUser, error) {
	key := strings.ToLower(handle)
	s.mu.Lock()
	c, ok := s.users[key]
	s.mu.Unlock()
	if ok && time.Since(c.at) < xUserTTL {
		return c.user, nil
	}
	body, err := s.graphql(ctx, "UserByScreenName",
		map[string]any{"screen_name": handle, "withGrokTranslatedBio": false},
		xUserFeatures, map[string]bool{"withAuxiliaryUserLabels": true})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			User struct {
				Result *gqlUser `json:"result"`
			} `json:"user"`
		} `json:"data"`
		Errors []gqlError `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("X returned invalid JSON: %v", err)
	}
	u := resp.Data.User.Result
	switch {
	case u == nil && len(resp.Errors) > 0:
		return nil, classifyGraphQLErrors(resp.Errors)
	case u == nil || u.RestID == "" && u.Typename != "UserUnavailable":
		return nil, fmt.Errorf("@%s: %w", handle, ErrXNotFound)
	case u.Typename == "UserUnavailable":
		reason := strings.ToLower(u.Reason + " " + u.Message)
		if strings.Contains(reason, "suspend") {
			return nil, fmt.Errorf("@%s is suspended", handle)
		}
		return nil, fmt.Errorf("@%s is unavailable", handle)
	}
	s.mu.Lock()
	s.users[key] = xCachedUser{user: u, at: time.Now()}
	s.mu.Unlock()
	return u, nil
}

// timelineQuery names the operation and variables of a timeline type.
func timelineQuery(kind, userID string, count int, cursor string) (string, map[string]any) {
	v := map[string]any{"count": count}
	if cursor != "" {
		v["cursor"] = cursor
	}
	op := "UserTweets"
	switch kind {
	case "media":
		op = "UserMedia"
		v["includePromotedContent"] = false
		v["withClientEventToken"] = false
		v["withBirdwatchNotes"] = false
		v["withVoice"] = true
	case "with_replies":
		op = "UserTweetsAndReplies"
		v["includePromotedContent"] = true
		v["withCommunity"] = true
		v["withVoice"] = true
	case "likes":
		op = "Likes"
		v["includePromotedContent"] = false
		v["withClientEventToken"] = false
		v["withBirdwatchNotes"] = false
		v["withVoice"] = true
	case "bookmarks":
		op = "Bookmarks"
		v["includePromotedContent"] = true
		return op, v
	default: // timeline, tweets
		v["includePromotedContent"] = true
		v["withQuickPromoteEligibilityTweetFields"] = true
		v["withVoice"] = true
	}
	v["userId"] = userID
	return op, v
}

func (s *xSession) timelinePage(ctx context.Context, kind, userID string, count int, cursor string) (*timelinePage, error) {
	op, vars := timelineQuery(kind, userID, count, cursor)
	body, err := s.graphql(ctx, op, vars, xTimelineFeatures, map[string]bool{"withArticlePlainText": false})
	if err != nil {
		return nil, err
	}
	return parseTimelinePage(body)
}

// A media timeline with retweets reads two timelines at once — the media
// tab, which leaves retweets out, and the retweets in the tweets tab — so
// its cursor holds one cursor for each, and whether each is done.
type combinedCursor struct {
	Media     string `json:"m,omitempty"`
	Tweets    string `json:"t,omitempty"`
	MediaDone bool   `json:"md,omitempty"`
	TweetDone bool   `json:"td,omitempty"`
}

const combinedPrefix = "xdl1:"

func (c combinedCursor) encode() string {
	b, _ := json.Marshal(c)
	return combinedPrefix + base64.RawURLEncoding.EncodeToString(b)
}

func decodeCombinedCursor(s string) combinedCursor {
	if !strings.HasPrefix(s, combinedPrefix) {
		return combinedCursor{Media: s} // a media-tab cursor from before
	}
	var c combinedCursor
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, combinedPrefix)); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

// fetchTimeline reads one page (one per timeline for a combined one) and
// keeps the tweets that belong to it.
func fetchTimeline(ctx context.Context, s *xSession, req TimelineRequest, kind string) (*richResponse, error) {
	count := req.BatchSize
	if count <= 0 || count > 100 {
		count = 100
	}
	if count < 20 {
		count = 20
	}

	out := &richResponse{}
	var user *gqlUser
	if kind != "bookmarks" {
		handle := cleanUsername(req.Username)
		if handle == "" {
			return nil, errors.New("no account given")
		}
		u, err := s.lookupUser(ctx, handle)
		if err != nil {
			return nil, err
		}
		user = u
		out.Account = toRichAccount(u)
	}

	ownTweet := func(t richTweet) bool {
		if user == nil || kind == "likes" {
			return true
		}
		if t.IsRepost {
			return t.RepostedBy != nil && (t.RepostedBy.UserID == user.RestID || strings.EqualFold(t.RepostedBy.Username, user.screenName()))
		}
		return t.Author.UserID == user.RestID || strings.EqualFold(t.Author.Username, user.screenName())
	}
	keep := func(t richTweet) bool {
		if !ownTweet(t) {
			return false // the tweets a reply answers, in replies' conversations
		}
		if t.IsRepost && !req.Retweets && kind != "likes" && kind != "bookmarks" {
			return false
		}
		return true
	}

	if kind == "media" && req.Retweets {
		return fetchMediaWithRetweets(ctx, s, req, user, count, out)
	}

	userID := ""
	if user != nil {
		userID = user.RestID
	}
	page, err := s.timelinePage(ctx, kind, userID, count, req.Cursor)
	if err != nil {
		return nil, err
	}
	for _, t := range page.tweets {
		if keep(t) {
			out.Tweets = append(out.Tweets, t)
		}
	}
	out.NextCursor = page.next
	out.HasMore = page.next != "" && page.next != req.Cursor && len(page.tweets) > 0
	if len(page.tweets) == 0 && req.Cursor == "" && user != nil && out.Account.Protected {
		return nil, fmt.Errorf("@%s's posts are protected: follow it with the account whose auth token XDL uses", user.screenName())
	}
	return out, nil
}

func fetchMediaWithRetweets(ctx context.Context, s *xSession, req TimelineRequest, user *gqlUser, count int, out *richResponse) (*richResponse, error) {
	c := decodeCombinedCursor(req.Cursor)
	next := c
	if !c.MediaDone {
		page, err := s.timelinePage(ctx, "media", user.RestID, count, c.Media)
		if err != nil {
			return nil, err
		}
		out.Tweets = append(out.Tweets, page.tweets...)
		next.Media = page.next
		next.MediaDone = page.next == "" || page.next == c.Media || len(page.tweets) == 0
	}
	if !c.TweetDone {
		page, err := s.timelinePage(ctx, "tweets", user.RestID, count, c.Tweets)
		if err != nil {
			return nil, err
		}
		for _, t := range page.tweets {
			if t.IsRepost && len(t.Media) > 0 && t.RepostedBy != nil && t.RepostedBy.UserID == user.RestID {
				out.Tweets = append(out.Tweets, t)
			}
		}
		next.Tweets = page.next
		next.TweetDone = page.next == "" || page.next == c.Tweets || len(page.tweets) == 0
	}
	out.HasMore = !next.MediaDone || !next.TweetDone
	if out.HasMore {
		out.NextCursor = next.encode()
	}
	return out, nil
}
