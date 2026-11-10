//go:build live

package backend

// Tests against x.com itself, run by hand:
//
//	go test -tags live -run Live -v ./backend/
//
// With XDL_AUTH_TOKEN set they read timelines as that account; without it
// they use guest access, which X limits to profiles.

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestLiveSigner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := getXSession("", "")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := newTxSigner(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("animation key %q, id %s", signer.animationKey, signer.id("GET", "/i/api/graphql/x/UserByScreenName", time.Now()))
}

func TestLiveQueryIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	token := os.Getenv("XDL_AUTH_TOKEN")
	if token == "" {
		t.Skip("x.com serves the classic web client, which holds the query IDs, signed in only: set XDL_AUTH_TOKEN")
	}
	s, err := getXSession(token, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.refreshQueryIDs(ctx); err != nil {
		t.Fatal(err)
	}
	for op, id := range s.queryIDs {
		if id != xDefaultQueryIDs[op] {
			t.Logf("%s: X now uses %s (built in: %s)", op, id, xDefaultQueryIDs[op])
		}
	}
}

func TestLiveTimeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	token := os.Getenv("XDL_AUTH_TOKEN")
	handle := os.Getenv("XDL_LIVE_ACCOUNT")
	if handle == "" {
		handle = "NASA"
	}
	s, err := getXSession(token, "")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.lookupUser(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	acc := toRichAccount(u)
	t.Logf("@%s: %s, %d followers, %d media", acc.Username, acc.Name, acc.Followers, acc.MediaCount)
	if token == "" {
		return
	}
	for _, kind := range []string{"media", "tweets"} {
		resp, err := ExtractTimeline(ctx, TimelineRequest{
			Username: handle, AuthToken: token, TimelineType: kind, BatchSize: 20,
			IncludePhotos: true, IncludeVideos: true, IncludeGifs: true,
		})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		t.Logf("%s: %d entries, more: %v", kind, len(resp.Timeline), resp.Metadata.HasMore)
		if len(resp.Timeline) == 0 {
			t.Errorf("%s: no entries", kind)
		}
		if resp.Metadata.HasMore {
			next, err := ExtractTimeline(ctx, TimelineRequest{
				Username: handle, AuthToken: token, TimelineType: kind, BatchSize: 20, Cursor: resp.Metadata.Cursor,
				IncludePhotos: true, IncludeVideos: true, IncludeGifs: true,
			})
			if err != nil {
				t.Fatalf("%s page 2: %v", kind, err)
			}
			t.Logf("%s page 2: %d entries", kind, len(next.Timeline))
		}
	}
}

// TestLiveTransactionIDChecked sends one guest request without a
// transaction ID, with a garbage one and with XDL's, to see whether X checks
// it and accepts XDL's.
func TestLiveTransactionIDChecked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := getXSession("", "")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := newTxSigner(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	gt, err := s.activateGuest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := "/i/api/graphql/" + xDefaultQueryIDs["UserByScreenName"] + "/UserByScreenName"
	q := url.Values{}
	q.Set("variables", `{"screen_name":"NASA","withGrokTranslatedBio":false}`)
	q.Set("features", mustJSON(xUserFeatures))
	q.Set("fieldToggles", `{"withAuxiliaryUserLabels":true}`)
	for _, c := range []struct{ name, tid string }{
		{"none", ""},
		{"garbage", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{"xdl", signer.id("GET", path, time.Now())},
	} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://x.com"+path+"?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+xWebBearer)
		req.Header.Set("User-Agent", xUserAgent)
		req.Header.Set("X-Guest-Token", gt)
		req.Header.Set("X-Csrf-Token", s.csrf)
		req.Header.Set("Cookie", "ct0="+s.csrf+"; gt="+gt)
		if c.tid != "" {
			req.Header.Set("X-Client-Transaction-Id", c.tid)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		t.Logf("%-8s → %s", c.name, resp.Status)
	}
}
