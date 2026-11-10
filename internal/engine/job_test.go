package engine

import "testing"

func TestNormalizeHandle(t *testing.T) {
	for in, want := range map[string]string{
		"@nasa": "nasa", "nasa": "nasa", " https://x.com/nasa/media ": "nasa",
		"x.com/nasa": "nasa", "https://twitter.com/NASA?s=20": "NASA",
	} {
		if got := NormalizeHandle(in); got != want {
			t.Errorf("NormalizeHandle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	r := Request{Username: "https://x.com/nasa"}
	if err := r.Validate(); err != nil || r.Username != "nasa" || r.TimelineType != "media" || r.Mode != ModeDownload || r.Source != "api" {
		t.Fatalf("got %+v, %v", r, err)
	}
	b := Request{TimelineType: "bookmarks"}
	if err := b.Validate(); err != nil || b.Username != "bookmarks" {
		t.Fatalf("bookmarks: %+v, %v", b, err)
	}
	for _, bad := range []Request{{}, {Username: "way_too_long_handle_x"}, {Username: "a", TimelineType: "nope"}, {Username: "a", Mode: "x"}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v validated", bad)
		}
	}
}
