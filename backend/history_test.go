package backend

import "testing"

func TestMediaKey(t *testing.T) {
	for _, tc := range []struct {
		url, typ string
		id       int64
		want     string
	}{
		{"https://pbs.twimg.com/media/HWkI-WUbEAAQ_nF?format=png&name=orig", "photo", 1, "photo:HWkI-WUbEAAQ_nF"},
		{"https://pbs.twimg.com/media/HWkI-WUbEAAQ_nF.jpg?name=large", "photo", 1, "photo:HWkI-WUbEAAQ_nF"},
		{"https://video.twimg.com/amplify_video/2119785537225846784/vid/avc1/1280x720/OOUx3Dic.mp4", "video", 1, "video:2119785537225846784"},
		{"https://video.twimg.com/amplify_video/2119785537225846784/vid/avc1/480x270/other.mp4?tag=1", "video", 1, "video:2119785537225846784"},
		{"https://video.twimg.com/ext_tw_video/123/pu/vid/avc1/720x1280/x.mp4", "video", 1, "video:123"},
		{"https://video.twimg.com/tweet_video/GbXyZ.mp4", "gif", 1, "gif:GbXyZ"},
		{"", "text", 42, "text:42"},
		{"https://example.com/a/b.png?x", "photo", 1, "url:example.com/a/b.png"},
	} {
		if got := MediaKey(tc.url, tc.id, tc.typ); got != tc.want {
			t.Errorf("MediaKey(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
