package backend

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A UserMedia page in the shape X serves: a grid module on the first page,
// a pinned tweet, a retweet, a promoted tweet and a bottom cursor.
const mediaPage = `{"data":{"user":{"result":{"__typename":"User","timeline":{"timeline":{"instructions":[
 {"type":"TimelineClearCache"},
 {"type":"TimelinePinEntry","entry":{"entryId":"tweet-1","content":{"entryType":"TimelineTimelineItem","itemContent":{"itemType":"TimelineTweet","tweet_results":{"result":{
   "__typename":"Tweet","rest_id":"1","source":"<a href=\"https://mobile.twitter.com\">Twitter Web App</a>",
   "core":{"user_results":{"result":{"__typename":"User","rest_id":"42","is_blue_verified":true,"core":{"name":"Alice","screen_name":"alice","created_at":"Mon Jan 01 00:00:00 +0000 2018"},"avatar":{"image_url":"https://pbs.twimg.com/profile_images/9/a_normal.jpg"},"legacy":{}}}},
   "views":{"count":"1234"},
   "legacy":{"id_str":"1","full_text":"pinned &amp; photo","created_at":"Tue Nov 10 11:43:42 +0000 2026","favorite_count":5,"retweet_count":1,"bookmark_count":2,
     "entities":{"hashtags":[{"text":"art"}]},
     "extended_entities":{"media":[{"id_str":"100","media_key":"3_100","type":"photo","media_url_https":"https://pbs.twimg.com/media/ABC.jpg",
       "original_info":{"width":2048,"height":1536},"sizes":{"large":{"w":2048,"h":1536,"resize":"fit"},"small":{"w":680,"h":510,"resize":"fit"}}}]}}}}}}}},
 {"type":"TimelineAddEntries","entries":[
  {"entryId":"profile-grid-0","content":{"entryType":"TimelineTimelineModule","items":[
   {"entryId":"profile-grid-0-tweet-2","item":{"itemContent":{"itemType":"TimelineTweet","tweet_results":{"result":{
     "__typename":"TweetWithVisibilityResults","tweet":{"rest_id":"2",
     "core":{"user_results":{"result":{"rest_id":"42","legacy":{"name":"Alice","screen_name":"alice"}}}},
     "legacy":{"id_str":"2","full_text":"a video","created_at":"Mon Nov 09 10:00:00 +0000 2026",
       "extended_entities":{"media":[{"id_str":"200","media_key":"7_200","type":"video","media_url_https":"https://pbs.twimg.com/ext_tw_video_thumb/200/pu/img/T.jpg",
         "original_info":{"width":1280,"height":720},
         "video_info":{"aspect_ratio":[16,9],"duration_millis":5000,"variants":[
           {"content_type":"application/x-mpegURL","url":"https://video.twimg.com/ext_tw_video/200/pu/pl/x.m3u8?tag=12"},
           {"content_type":"video/mp4","bitrate":832000,"url":"https://video.twimg.com/ext_tw_video/200/pu/vid/avc1/640x360/s.mp4?tag=12"},
           {"content_type":"video/mp4","bitrate":2176000,"url":"https://video.twimg.com/ext_tw_video/200/pu/vid/avc1/1280x720/l.mp4?tag=12"}]}}]}}}}}}}},
   {"entryId":"profile-grid-0-tweet-3","item":{"itemContent":{"itemType":"TimelineTweet","tweet_results":{"result":{"__typename":"TweetTombstone"}}}}}]}},
  {"entryId":"tweet-4","content":{"entryType":"TimelineTimelineItem","itemContent":{"itemType":"TimelineTweet","tweet_results":{"result":{
    "__typename":"Tweet","rest_id":"4",
    "core":{"user_results":{"result":{"rest_id":"42","core":{"screen_name":"alice","name":"Alice"}}}},
    "legacy":{"id_str":"4","full_text":"RT @bob: gif","created_at":"Sun Nov 08 10:00:00 +0000 2026",
      "retweeted_status_result":{"result":{"__typename":"Tweet","rest_id":"5",
        "core":{"user_results":{"result":{"rest_id":"7","core":{"screen_name":"bob","name":"Bob"}}}},
        "note_tweet":{"note_tweet_results":{"result":{"text":"a long gif post"}}},
        "legacy":{"id_str":"5","full_text":"gif","created_at":"Sat Nov 07 10:00:00 +0000 2026",
          "extended_entities":{"media":[{"id_str":"500","type":"animated_gif","media_url_https":"https://pbs.twimg.com/tweet_video_thumb/G.jpg",
            "video_info":{"variants":[{"content_type":"video/mp4","bitrate":0,"url":"https://video.twimg.com/tweet_video/G.mp4"}]}}]}}}}}}}}}},
  {"entryId":"promoted-tweet-9","content":{"entryType":"TimelineTimelineItem","itemContent":{"itemType":"TimelineTweet","promotedMetadata":{"advertiser_results":{}},"tweet_results":{"result":{"rest_id":"9","legacy":{"id_str":"9","full_text":"ad"}}}}}},
  {"entryId":"cursor-top-1","content":{"entryType":"TimelineTimelineCursor","cursorType":"Top","value":"TOP"}},
  {"entryId":"cursor-bottom-1","content":{"entryType":"TimelineTimelineCursor","cursorType":"Bottom","value":"NEXT"}}]}]}}}}}}`

func TestParseTimelinePage(t *testing.T) {
	page, err := parseTimelinePage([]byte(mediaPage))
	if err != nil {
		t.Fatal(err)
	}
	if page.next != "NEXT" {
		t.Errorf("next = %q", page.next)
	}
	if len(page.posts) != 3 {
		t.Fatalf("got %d posts, want 3 (pinned, video, repost; no tombstone or ad)", len(page.posts))
	}
	pinned, video, rt := page.posts[0], page.posts[1], page.posts[2]

	if pinned.ID != "1" || pinned.Author.Handle != "alice" || pinned.Source != "Twitter Web App" ||
		pinned.Views != 1234 || pinned.Likes != 5 || len(pinned.Hashtags) != 1 || !pinned.Author.Verified {
		t.Errorf("pinned = %+v", pinned)
	}
	photo := pinned.Media[0]
	if got := sizedImage(photo.Image, "orig"); got != "https://pbs.twimg.com/media/ABC?format=jpg&name=orig" {
		t.Errorf("orig = %q", got)
	}
	v := video.Media[0]
	if video.ID != "2" || v.Kind != "video" || v.DurationMS != 5000 {
		t.Errorf("video = %+v", video)
	}
	if got := pickClip(v, "highest"); got != "https://video.twimg.com/ext_tw_video/200/pu/vid/avc1/1280x720/l.mp4" {
		t.Errorf("highest = %q", got)
	}
	if got := pickClip(v, "360"); !strings.Contains(got, "640x360") {
		t.Errorf("360p = %q", got)
	}
	if got := pickClip(v, "lowest"); !strings.Contains(got, "640x360") {
		t.Errorf("lowest = %q", got)
	}
	if rt.ID != "5" || rt.Author.Handle != "bob" || rt.RepostedBy == nil ||
		rt.RepostedBy.Handle != "alice" || rt.Text != "a long gif post" || rt.Media[0].Kind != "animated_gif" {
		t.Errorf("repost = %+v", rt)
	}

	all := entryOptions{photos: true, videos: true, gifs: true, imageSize: "small"}
	entries := timelineEntries(page.posts, all)
	if len(entries) != 3 || entries[0].Extension != "jpg" || entries[1].Extension != "mp4" || !entries[2].IsRetweet {
		t.Fatalf("entries = %+v", entries)
	}
	if e := entries[0]; e.URL != "https://pbs.twimg.com/media/ABC?format=jpg&name=small" ||
		e.Thumbnail != "https://pbs.twimg.com/media/ABC?format=jpg&name=360x360" || e.TweetID != 1 ||
		e.AuthorUsername != "alice" || e.FavoriteCount != 5 || e.Width != 2048 {
		t.Errorf("photo entry = %+v", e)
	}
	if k := MediaKey(entries[1].URL, 2, "video"); k != "video:200" {
		t.Errorf("history key = %q", k)
	}
	if n := len(timelineEntries(page.posts, entryOptions{videos: true})); n != 1 {
		t.Errorf("videos only: %d entries", n)
	}
	text := []post{{ID: "7", Text: "hello"}, {ID: "8", Text: "  "}}
	if es := timelineEntries(text, entryOptions{text: true}); len(es) != 1 || es[0].Type != "text" || es[0].Extension != "txt" {
		t.Errorf("text entries = %+v", es)
	}
}

func TestParseTimelineErrors(t *testing.T) {
	_, err := parseTimelinePage([]byte(`{"errors":[{"message":"Could not authenticate you","code":32}]}`))
	if err != ErrXAuth {
		t.Errorf("err = %v", err)
	}
	_, err = parseTimelinePage([]byte(`{"errors":[{"message":"Rate limit exceeded","code":88}]}`))
	if _, ok := err.(*XRateLimitError); !ok {
		t.Errorf("err = %v", err)
	}
	page, err := parseTimelinePage([]byte(`{"data":{"user":{"result":{"timeline_v2":{"timeline":{"instructions":[]}}}}}}`))
	if err != nil || len(page.posts) != 0 {
		t.Errorf("empty timeline: %v %v", page, err)
	}
}

func TestProfile(t *testing.T) {
	u := &gqlUser{RestID: "42", IsBlueVerified: true}
	u.Core.ScreenName, u.Core.Name, u.Core.CreatedAt = "alice", "Alice", "Mon Jan 01 00:00:00 +0000 2018"
	u.Avatar.ImageURL = "https://pbs.twimg.com/profile_images/9/a_normal.jpg"
	u.Legacy.ProfileBannerURL = "https://pbs.twimg.com/profile_banners/42/1"
	u.Legacy.FollowersCount, u.Legacy.MediaCount = 10, 3
	u.Privacy.Protected = true
	p := u.profile()
	if p.Handle != "alice" || p.Avatar != "https://pbs.twimg.com/profile_images/9/a.jpg" || !p.Protected || p.Followers != 10 {
		t.Errorf("profile = %+v", p)
	}
	if p.AvatarSizes[0].URL != "https://pbs.twimg.com/profile_images/9/a_mini.jpg" {
		t.Errorf("avatar sizes = %+v", p.AvatarSizes)
	}
	// The 2026 shape: counts and the profile out of legacy.
	n := &gqlUser{RestID: "43"}
	n.Core.ScreenName = "bob"
	n.RelationshipCounts.Followers, n.TweetCounts.MediaTweets = 92, 7
	n.ProfileBio.Description, n.Website.URL = "bio", "https://example.com"
	if b := n.profile(); b.Followers != 92 || b.MediaCount != 7 || b.Bio != "bio" || b.URL != "https://example.com" {
		t.Errorf("2026 profile = %+v", b)
	}
	info := p.info("alice")
	if info.Name != "alice" || info.Nick != "Alice" || info.Date != "Mon Jan 01 00:00:00 +0000 2018" || !info.BlueVerified {
		t.Errorf("info = %+v", info)
	}
	if info := (profile{}).info("carol"); info.Name != "carol" || info.Nick != "carol" {
		t.Errorf("fallback info = %+v", info)
	}
}

func TestCombinedCursor(t *testing.T) {
	c := combinedCursor{Media: "m1", Tweets: "t1", TweetDone: true}
	if got := decodeCombinedCursor(c.encode()); got != c {
		t.Errorf("round trip = %+v", got)
	}
	if got := decodeCombinedCursor("DAABCgAB"); got.Media != "DAABCgAB" || got.Tweets != "" {
		t.Errorf("plain cursor = %+v", got)
	}
}

func TestTimelineQuery(t *testing.T) {
	op, v := timelineQuery("media", "42", 100, "C")
	if op != "UserMedia" || v["userId"] != "42" || v["cursor"] != "C" || v["count"] != 100 {
		t.Errorf("media = %s %v", op, v)
	}
	op, v = timelineQuery("bookmarks", "", 20, "")
	if _, ok := v["userId"]; op != "Bookmarks" || ok {
		t.Errorf("bookmarks = %s %v", op, v)
	}
	for kind, want := range map[string]string{"timeline": "UserTweets", "tweets": "UserTweets", "with_replies": "UserTweetsAndReplies", "likes": "Likes"} {
		if op, _ := timelineQuery(kind, "1", 20, ""); op != want {
			t.Errorf("%s = %s", kind, op)
		}
	}
}

func TestTxHelpers(t *testing.T) {
	for in, want := range map[float64]string{0: "", 1: "1", 0.5: ".8", 0.25: ".4", 255: "FF", 0.87: ".DEB851EB851EB8"} {
		if got := floatToHex(in); got != want {
			t.Errorf("floatToHex(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]float64{2.5: 3, 2.4: 2, -2.5: -2, -2.6: -3, 0: 0} {
		if got := jsRound(in); got != want {
			t.Errorf("jsRound(%v) = %v", in, got)
		}
	}
	linear := []float64{0, 0, 1, 1}
	if got := cubicValue(linear, 0.5); got < 0.49 || got > 0.51 {
		t.Errorf("linear ease at 0.5 = %v", got)
	}
}

func TestTransactionID(t *testing.T) {
	frame := func(d string) string {
		return `<svg id="loading-x-anim-0"><g><path d="M0 0"/><path d="` + d + `"/></g></svg>`
	}
	// "M 10,30 " is cut (the first 9 bytes), then rows split at C.
	d := "M 10,30 C 10 20 30 40 50 60 70 80 90 100 110 C 255 0 0 0 0 255 128 10 200 30 220"
	frames := []string{frame(d), frame(d), frame(d), frame(d)}
	key := []byte{0, 1, 1, 3, 4, 8, 6, 7, 1, 3, 5, 7, 9, 11, 13, 15}
	animationKey, err := computeAnimationKey(key, 2, []int{9, 10, 11}, frames)
	if err != nil {
		t.Fatal(err)
	}
	if animationKey == "" || strings.ContainsAny(animationKey, ".-") {
		t.Errorf("animation key = %q", animationKey)
	}

	signer := &txSigner{keyBytes: key, animationKey: animationKey}
	now := time.Unix(txEpoch+1000, 0)
	id := signer.id("GET", "/i/api/graphql/x/UserMedia", now)
	raw, err := base64.RawStdEncoding.DecodeString(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1+len(key)+4+16+1 {
		t.Fatalf("length %d", len(raw))
	}
	r := raw[0]
	for i, b := range key {
		if raw[1+i]^r != b {
			t.Fatalf("key byte %d", i)
		}
	}
	elapsed := uint32(raw[1+len(key)]^r) | uint32(raw[2+len(key)]^r)<<8 | uint32(raw[3+len(key)]^r)<<16 | uint32(raw[4+len(key)]^r)<<24
	if elapsed != 1000 {
		t.Errorf("time = %d", elapsed)
	}
	if raw[len(raw)-1]^r != txExtraByte {
		t.Error("extra byte")
	}
}

func TestOndemandScriptURL(t *testing.T) {
	html := `...,20113:"ondemand.s",20114:"x"...{20113:"6f8f1d2c",20114:"aa"}`
	if got, _ := ondemandScriptURL(html); got != "https://abs.twimg.com/responsive-web/client-web/ondemand.s.6f8f1d2ca.js" {
		t.Errorf("chunk map: %q", got)
	}
	if got, _ := ondemandScriptURL(`{"ondemand.s":"abc123"}`); got != "https://abs.twimg.com/responsive-web/client-web/ondemand.s.abc123a.js" {
		t.Errorf("legacy: %q", got)
	}
	key, err := verificationKey(`<meta name="twitter-site-verification" content="AAEC"/>`)
	if err != nil || key != "AAEC" {
		t.Errorf("key = %q %v", key, err)
	}
}

func TestRemoveXtractor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := legacyDir()
	if !strings.HasPrefix(dir, home) {
		t.Skipf("the old library folder %s is not under HOME", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xtractor", "ffmpeg", "extractor.sha256", "accounts.db"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removeXtractor()
	for _, name := range []string{"xtractor", "ffmpeg"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s is still there", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "accounts.db")); err != nil {
		t.Error("the library was removed")
	}
}
