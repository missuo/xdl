package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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

// parsedPage is one page as X sends it: its posts in order and the cursor
// of the next.
type parsedPage struct {
	posts []post
	next  string
}

func parseTimelinePage(body []byte) (*parsedPage, error) {
	var resp gqlTimelineResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("X returned invalid JSON: %v", err)
	}
	instructions := resp.instructions()
	if instructions == nil && len(resp.Errors) > 0 {
		return nil, classifyGraphQLErrors(resp.Errors)
	}
	page := &parsedPage{}
	add := func(id string, c *gqlItemContent) {
		if c == nil || len(c.TweetResults.Result) == 0 || len(c.PromotedMetadata) > 0 && string(c.PromotedMetadata) != "null" {
			return
		}
		if strings.HasPrefix(id, "promoted-") {
			return
		}
		var t gqlTweet
		if err := json.Unmarshal(c.TweetResults.Result, &t); err != nil {
			return // a post in a shape XDL can't read is skipped, not fatal
		}
		if p, ok := t.post(); ok {
			page.posts = append(page.posts, p)
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

// post reads a post, or the original of a repost with RepostedBy set; ok
// is false for a tombstone or a withheld post.
func (raw *gqlTweet) post() (post, bool) {
	t := raw.unwrap()
	if t == nil || t.Legacy == nil {
		return post{}, false
	}
	if orig := t.Legacy.RetweetedStatusResult.Result.unwrap(); orig != nil && orig.Legacy != nil {
		p, ok := orig.post()
		if !ok {
			return post{}, false
		}
		by := t.Core.UserResults.Result.person()
		p.RepostedBy = &by
		return p, true
	}
	l := t.Legacy
	p := post{
		ID:        firstNonEmpty(l.IDStr, t.RestID),
		Text:      firstNonEmpty(t.NoteTweet.NoteTweetResults.Result.Text, l.FullText),
		Lang:      l.Lang,
		CreatedAt: l.CreatedAt,
		Source:    html.UnescapeString(reHTMLTag.ReplaceAllString(t.Source, "")),
		Author:    t.Core.UserResults.Result.person(),
		Reply:     l.InReplyToStatusIDStr != "",
		Quote:     l.IsQuoteStatus,
		Sensitive: l.PossiblySensitive,
		Likes:     l.FavoriteCount,
		Reposts:   l.RetweetCount,
		Replies:   l.ReplyCount,
		Quotes:    l.QuoteCount,
		Bookmarks: l.BookmarkCount,
	}
	p.Views, _ = strconv.Atoi(t.Views.Count)
	for _, h := range l.Entities.Hashtags {
		p.Hashtags = append(p.Hashtags, h.Text)
	}
	for _, m := range l.ExtendedEntities.Media {
		p.Media = append(p.Media, m.attachment())
	}
	return p, true
}

func (u *gqlUser) person() person {
	if u == nil {
		return person{}
	}
	return person{
		ID:       u.RestID,
		Handle:   u.screenName(),
		Name:     u.name(),
		Avatar:   u.avatar(),
		Verified: u.Verification.Verified || u.Legacy.Verified || u.IsBlueVerified,
	}
}

var reVideoResolution = regexp.MustCompile(`/(\d+x\d+)/`)

func (m gqlMedia) attachment() attachment {
	a := attachment{
		Kind:       m.Type,
		ID:         m.IDStr,
		Key:        m.MediaKey,
		Image:      m.MediaURLHTTPS,
		Width:      m.OriginalInfo.Width,
		Height:     m.OriginalInfo.Height,
		DurationMS: m.VideoInfo.DurationMillis,
		AltText:    m.ExtAltText,
	}
	for _, v := range m.VideoInfo.Variants {
		u, _, _ := strings.Cut(v.URL, "?tag=")
		c := clip{ContentType: v.ContentType, URL: u, Bitrate: v.Bitrate}
		if r := reVideoResolution.FindStringSubmatch(u); r != nil {
			c.Resolution = r[1]
		}
		a.Clips = append(a.Clips, c)
	}
	return a
}

func (u *gqlUser) profile() profile {
	p := profile{
		ID:         u.RestID,
		Handle:     u.screenName(),
		Name:       u.name(),
		Bio:        firstNonEmpty(u.ProfileBio.Description, u.Legacy.Description),
		Banner:     firstNonEmpty(u.Banner.ImageURL, u.Legacy.ProfileBannerURL),
		Location:   firstNonEmpty(u.Location.Location, u.Legacy.Location),
		URL:        firstNonEmpty(u.Website.URL, u.Legacy.URL),
		CreatedAt:  firstNonEmpty(u.Core.CreatedAt, u.Legacy.CreatedAt),
		Followers:  max(u.RelationshipCounts.Followers, u.Legacy.FollowersCount),
		Following:  max(u.RelationshipCounts.Following, u.Legacy.FriendsCount),
		Posts:      max(u.TweetCounts.Tweets, u.Legacy.StatusesCount),
		MediaCount: max(u.TweetCounts.MediaTweets, u.Legacy.MediaCount),
		Protected:  u.Privacy.Protected || u.Legacy.Protected,
		Verified:   u.Verification.Verified || u.Legacy.Verified,
		Blue:       u.IsBlueVerified,
	}
	if urls := u.Legacy.Entities.URL.URLs; u.Website.URL == "" && len(urls) > 0 && urls[0].ExpandedURL != "" {
		p.URL = urls[0].ExpandedURL
	}
	// X serves profile pictures in fixed sizes: name_normal.jpg,
	// name_400x400.jpg… and the original without a suffix.
	if avatar := u.avatar(); avatar != "" {
		p.Avatar = strings.Replace(avatar, "_normal.", ".", 1)
		if dot := strings.LastIndexByte(p.Avatar, '.'); dot > strings.LastIndexByte(p.Avatar, '/') {
			for _, size := range []string{"mini", "normal", "bigger", "200x200", "400x400"} {
				p.AvatarSizes = append(p.AvatarSizes, ImageVariant{Name: size, URL: p.Avatar[:dot] + "_" + size + p.Avatar[dot:]})
			}
		}
		p.AvatarSizes = append(p.AvatarSizes, ImageVariant{Name: "orig", URL: p.Avatar})
	}
	if p.Banner != "" {
		for _, size := range []string{"300x100", "600x200", "1080x360", "1500x500"} {
			p.BannerSizes = append(p.BannerSizes, ImageVariant{Name: size, URL: p.Banner + "/" + size})
		}
	}
	return p
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

func (s *xSession) timelinePage(ctx context.Context, kind, userID string, count int, cursor string) (*parsedPage, error) {
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
// keeps the posts that belong to it.
func fetchTimeline(ctx context.Context, s *xSession, req TimelineRequest, kind string) (*timelinePage, error) {
	count := req.BatchSize
	if count <= 0 || count > 100 {
		count = 100
	}
	count = max(count, 20)

	out := &timelinePage{}
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
		out.account = u.profile()
	}
	if kind == "media" && req.Retweets {
		return fetchMediaWithRetweets(ctx, s, user, count, req.Cursor, out)
	}

	// A replies timeline carries the posts they answer; likes and
	// bookmarks are other people's posts by nature.
	byUser := func(p post) bool {
		if user == nil || kind == "likes" {
			return true
		}
		who := p.Author
		if p.RepostedBy != nil {
			who = *p.RepostedBy
		}
		return who.ID == user.RestID || strings.EqualFold(who.Handle, user.screenName())
	}
	userID := ""
	if user != nil {
		userID = user.RestID
	}
	pg, err := s.timelinePage(ctx, kind, userID, count, req.Cursor)
	if err != nil {
		return nil, err
	}
	for _, p := range pg.posts {
		if !byUser(p) {
			continue
		}
		if p.RepostedBy != nil && !req.Retweets && kind != "likes" && kind != "bookmarks" {
			continue
		}
		out.posts = append(out.posts, p)
	}
	out.next = pg.next
	out.more = pg.next != "" && pg.next != req.Cursor && len(pg.posts) > 0
	if len(pg.posts) == 0 && req.Cursor == "" && user != nil && out.account.Protected {
		return nil, fmt.Errorf("@%s's posts are protected: follow it with the account whose auth token XDL uses", user.screenName())
	}
	return out, nil
}

// fetchMediaWithRetweets reads the media tab, which leaves reposts out,
// and the reposts with media of the posts tab, a page of each.
func fetchMediaWithRetweets(ctx context.Context, s *xSession, user *gqlUser, count int, cursor string, out *timelinePage) (*timelinePage, error) {
	c := decodeCombinedCursor(cursor)
	next := c
	if !c.MediaDone {
		pg, err := s.timelinePage(ctx, "media", user.RestID, count, c.Media)
		if err != nil {
			return nil, err
		}
		out.posts = append(out.posts, pg.posts...)
		next.Media = pg.next
		next.MediaDone = pg.next == "" || pg.next == c.Media || len(pg.posts) == 0
	}
	if !c.TweetDone {
		pg, err := s.timelinePage(ctx, "tweets", user.RestID, count, c.Tweets)
		if err != nil {
			return nil, err
		}
		for _, p := range pg.posts {
			if p.RepostedBy != nil && p.RepostedBy.ID == user.RestID && len(p.Media) > 0 {
				out.posts = append(out.posts, p)
			}
		}
		next.Tweets = pg.next
		next.TweetDone = pg.next == "" || pg.next == c.Tweets || len(pg.posts) == 0
	}
	out.more = !next.MediaDone || !next.TweetDone
	if out.more {
		out.next = next.encode()
	}
	return out, nil
}
