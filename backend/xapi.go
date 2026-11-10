package backend

// XDL's own client for X's web GraphQL API, in place of the xtractor binary
// it used to run (which uploaded the auth token to its author). It talks to
// x.com and abs.twimg.com only, and sends the auth token to x.com only.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// The bearer token of X's web client, the same for every visitor.
	xWebBearer = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
	xUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
	xHome      = "https://x.com/"
	xAPI       = "https://x.com/i/api"

	xMaxResponse = 64 << 20
	xUserTTL     = 10 * time.Minute
	xSignerTTL   = time.Hour
)

// The operations' query IDs in the X web client of October 2026. X retires
// them now and then; refreshQueryIDs reads the current ones from the web
// client's main.js when one stops working.
var xDefaultQueryIDs = map[string]string{
	"UserByScreenName":     "ck5KkZ8t5cOmoLssopN99Q",
	"UserTweets":           "E8Wq-_jFSaU7hxVcuOPR9g",
	"UserTweetsAndReplies": "-O3QOHrVn1aOm_cF5wyTCQ",
	"UserMedia":            "jCRhbOzdgOHp6u9H4g2tEg",
	"Likes":                "TGEKkJG_meudeaFcqaxM-Q",
	"Bookmarks":            "pLtjrO4ubNh996M_Cubwsg",
}

var xTimelineFeatures = map[string]bool{
	"payments_enabled":   false,
	"rweb_xchat_enabled": false,
	"profile_label_improvements_pcf_label_in_post_enabled":                    true,
	"rweb_tipjar_consumption_enabled":                                         true,
	"verified_phone_label_enabled":                                            false,
	"creator_subscriptions_tweet_preview_api_enabled":                         true,
	"responsive_web_graphql_timeline_navigation_enabled":                      true,
	"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
	"premium_content_api_read_enabled":                                        false,
	"communities_web_enable_tweet_community_results_fetch":                    true,
	"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
	"responsive_web_grok_analyze_button_fetch_trends_enabled":                 false,
	"responsive_web_grok_analyze_post_followups_enabled":                      true,
	"responsive_web_jetfuel_frame":                                            true,
	"responsive_web_grok_share_attachment_enabled":                            true,
	"articles_preview_enabled":                                                true,
	"responsive_web_edit_tweet_api_enabled":                                   true,
	"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
	"view_counts_everywhere_api_enabled":                                      true,
	"longform_notetweets_consumption_enabled":                                 true,
	"responsive_web_twitter_article_tweet_consumption_enabled":                true,
	"tweet_awards_web_tipping_enabled":                                        false,
	"responsive_web_grok_show_grok_translated_post":                           false,
	"responsive_web_grok_analysis_button_from_backend":                        true,
	"creator_subscriptions_quote_tweet_preview_enabled":                       false,
	"freedom_of_speech_not_reach_fetch_enabled":                               true,
	"standardized_nudges_misinfo":                                             true,
	"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
	"longform_notetweets_rich_text_read_enabled":                              true,
	"longform_notetweets_inline_media_enabled":                                true,
	"responsive_web_grok_image_annotation_enabled":                            true,
	"responsive_web_grok_imagine_annotation_enabled":                          true,
	"responsive_web_grok_community_note_auto_translation_is_enabled":          false,
	"responsive_web_enhance_cards_enabled":                                    false,
}

var xUserFeatures = map[string]bool{
	"hidden_profile_subscriptions_enabled":                              true,
	"payments_enabled":                                                  false,
	"rweb_xchat_enabled":                                                false,
	"profile_label_improvements_pcf_label_in_post_enabled":              true,
	"rweb_tipjar_consumption_enabled":                                   true,
	"verified_phone_label_enabled":                                      false,
	"highlights_tweets_tab_ui_enabled":                                  true,
	"responsive_web_twitter_article_notes_tab_enabled":                  true,
	"subscriptions_feature_can_gift_premium":                            true,
	"creator_subscriptions_tweet_preview_api_enabled":                   true,
	"responsive_web_graphql_skip_user_profile_image_extensions_enabled": false,
	"responsive_web_graphql_timeline_navigation_enabled":                true,
	"subscriptions_verification_info_is_identity_verified_enabled":      true,
	"subscriptions_verification_info_verified_since_enabled":            true,
}

// Errors the GUI, the CLI and the engine tell apart.
var (
	ErrXAuth        = errors.New("X rejected the auth token — it is invalid, expired or was logged out; set a new one")
	ErrXRateLimited = errors.New("X rate limit reached")
	ErrXNotFound    = errors.New("account not found")
)

// XRateLimitError says when X lifts a rate limit.
type XRateLimitError struct{ Reset time.Time }

func (e *XRateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "X rate limit reached — wait 15 minutes and try again"
	}
	wait := time.Until(e.Reset).Round(time.Minute)
	if wait < time.Minute {
		wait = time.Minute
	}
	return fmt.Sprintf("X rate limit reached — try again in %s (at %s)", wait, e.Reset.Local().Format("15:04"))
}

func (e *XRateLimitError) Unwrap() error { return ErrXRateLimited }

// xSession holds what requests with one auth token share: the CSRF token,
// the transaction ID signer, the query IDs and the accounts looked up.
type xSession struct {
	client *http.Client
	token  string // the auth_token cookie, or "" for guest access

	signerInit  sync.Mutex // one signer setup at a time
	mu          sync.Mutex
	csrf        string
	guestToken  string
	signer      *txSigner
	signerAt    time.Time
	queryIDs    map[string]string
	refreshedAt time.Time
	users       map[string]xCachedUser
}

type xCachedUser struct {
	user *gqlUser
	at   time.Time
}

var xSessions = struct {
	sync.Mutex
	m map[string]*xSession
}{m: map[string]*xSession{}}

// getXSession returns the session for an auth token and proxy setting.
func getXSession(authToken, proxy string) (*xSession, error) {
	key := authToken + "\x00" + proxy
	xSessions.Lock()
	defer xSessions.Unlock()
	if s, ok := xSessions.m[key]; ok {
		return s, nil
	}
	client, err := CreateHTTPClient(proxy, 0)
	if err != nil {
		return nil, err
	}
	queryIDs := make(map[string]string, len(xDefaultQueryIDs))
	for op, id := range xDefaultQueryIDs {
		queryIDs[op] = id
	}
	s := &xSession{
		client:   client,
		token:    authToken,
		csrf:     randomHex(16),
		queryIDs: queryIDs,
		users:    map[string]xCachedUser{},
	}
	xSessions.m[key] = s
	return s, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// fetchPage GETs a page of x.com or abs.twimg.com without the auth token.
func (s *xSession) fetchPage(ctx context.Context, rawURL string) (string, error) {
	return s.fetchPageAs(ctx, rawURL, false)
}

// fetchPageAs GETs a page, signed in to x.com when signedIn is set: the
// classic web client, which holds the query IDs, is served signed in only.
func (s *xSession) fetchPageAs(ctx context.Context, rawURL string, signedIn bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if signedIn && s.token != "" && strings.HasPrefix(rawURL, xHome) {
		s.mu.Lock()
		req.Header.Set("Cookie", "auth_token="+s.token+"; ct0="+s.csrf)
		s.mu.Unlock()
	}
	req.Header.Set("User-Agent", xUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", rawURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, xMaxResponse))
	return string(body), err
}

// transactionID signs a request the way X's web client does, or returns ""
// when the signer can't be set up: X accepts most requests without one.
func (s *xSession) transactionID(ctx context.Context, method, path string) string {
	s.signerInit.Lock()
	s.mu.Lock()
	stale := s.signer == nil || time.Since(s.signerAt) > xSignerTTL
	// After a failure, try again a minute later, not on every request.
	due := s.signerAt.IsZero() || time.Since(s.signerAt) > time.Minute
	s.mu.Unlock()
	if stale && due {
		fresh, err := newTxSigner(ctx, s)
		s.mu.Lock()
		s.signerAt = time.Now()
		if err != nil {
			log.Printf("x: transaction IDs unavailable: %v", err)
		} else {
			s.signer = fresh
		}
		s.mu.Unlock()
	}
	s.signerInit.Unlock()

	s.mu.Lock()
	signer := s.signer
	s.mu.Unlock()
	if signer == nil {
		return ""
	}
	return signer.id(method, path, time.Now())
}

var (
	reMainJS  = regexp.MustCompile(`https://abs\.twimg\.com/responsive-web/client-web(?:-legacy)?/main\.[A-Za-z0-9_-]+\.js`)
	reQueryID = regexp.MustCompile(`queryId:"([A-Za-z0-9_-]+)",operationName:"(\w+)"`)
)

// refreshQueryIDs reads the current query IDs from the web client, at most
// every 10 minutes.
func (s *xSession) refreshQueryIDs(ctx context.Context) error {
	s.mu.Lock()
	if !s.refreshedAt.IsZero() && time.Since(s.refreshedAt) < 10*time.Minute {
		s.mu.Unlock()
		return errors.New("query IDs were refreshed recently")
	}
	s.refreshedAt = time.Now()
	s.mu.Unlock()

	home, err := s.fetchPageAs(ctx, xHome, true)
	if err != nil {
		return err
	}
	mainURL := reMainJS.FindString(home)
	if mainURL == "" {
		return errors.New("the classic web client's main.js was not found (x.com serves it signed in only)")
	}
	js, err := s.fetchPage(ctx, mainURL)
	if err != nil {
		return err
	}
	found := 0
	s.mu.Lock()
	for _, m := range reQueryID.FindAllStringSubmatch(js, -1) {
		if _, used := xDefaultQueryIDs[m[2]]; used {
			s.queryIDs[m[2]] = m[1]
			found++
		}
	}
	s.mu.Unlock()
	if found == 0 {
		return errors.New("no query IDs found in main.js")
	}
	return nil
}

func (s *xSession) activateGuest(ctx context.Context) (string, error) {
	s.mu.Lock()
	gt := s.guestToken
	s.mu.Unlock()
	if gt != "" {
		return gt, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.x.com/1.1/guest/activate.json", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+xWebBearer)
	req.Header.Set("User-Agent", xUserAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		GuestToken string `json:"guest_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.GuestToken == "" {
		return "", fmt.Errorf("guest access unavailable (%s)", resp.Status)
	}
	s.mu.Lock()
	s.guestToken = out.GuestToken
	s.mu.Unlock()
	return out.GuestToken, nil
}

type gqlError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
}

// graphql runs one GraphQL query and returns the response body.
func (s *xSession) graphql(ctx context.Context, op string, variables map[string]any, features map[string]bool, toggles map[string]bool) ([]byte, error) {
	retriedIDs := false
	for attempt := 0; ; attempt++ {
		s.mu.Lock()
		id := s.queryIDs[op]
		s.mu.Unlock()
		path := "/i/api/graphql/" + id + "/" + op
		q := url.Values{}
		q.Set("variables", mustJSON(variables))
		q.Set("features", mustJSON(features))
		if len(toggles) > 0 {
			q.Set("fieldToggles", mustJSON(toggles))
		}
		body, status, err := s.get(ctx, "https://x.com"+path+"?"+q.Encode(), path)
		if err != nil {
			return nil, err
		}
		switch {
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusForbidden && attempt == 0:
			continue // with the ct0 X just set
		case (status == http.StatusNotFound || status == http.StatusBadRequest && strings.Contains(string(body), "query")) && !retriedIDs:
			retriedIDs = true
			if rerr := s.refreshQueryIDs(ctx); rerr == nil {
				continue
			}
		}
		return nil, s.statusError(op, status, body)
	}
}

func (s *xSession) statusError(op string, status int, body []byte) error {
	var env struct {
		Errors []gqlError `json:"errors"`
	}
	_ = json.Unmarshal(body, &env)
	msg := ""
	if len(env.Errors) > 0 {
		msg = env.Errors[0].Message
	}
	switch status {
	case http.StatusUnauthorized:
		if s.token == "" {
			return fmt.Errorf("%s needs an auth token", op)
		}
		return ErrXAuth
	case http.StatusForbidden:
		if strings.Contains(strings.ToLower(msg), "authenticate") {
			return ErrXAuth
		}
	case http.StatusNotFound:
		if s.token == "" {
			return fmt.Errorf("%s needs an auth token: X shows it to signed-in accounts only", op)
		}
		return fmt.Errorf("%s: X no longer serves this query (404); XDL needs an update", op)
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
	}
	return fmt.Errorf("%s: X answered %d: %s", op, status, msg)
}

// get sends one authenticated API request, retrying a rate-limited one
// whose limit lifts within a minute.
func (s *xSession) get(ctx context.Context, rawURL, path string) ([]byte, int, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, 0, err
		}
		s.mu.Lock()
		csrf := s.csrf
		s.mu.Unlock()
		h := req.Header
		h.Set("Authorization", "Bearer "+xWebBearer)
		h.Set("User-Agent", xUserAgent)
		h.Set("Accept", "*/*")
		h.Set("Accept-Language", "en-US,en;q=0.9")
		h.Set("Content-Type", "application/json")
		h.Set("Referer", xHome)
		h.Set("Origin", "https://x.com")
		h.Set("X-Twitter-Active-User", "yes")
		h.Set("X-Twitter-Client-Language", "en")
		h.Set("X-Csrf-Token", csrf)
		h.Set("Sec-Fetch-Dest", "empty")
		h.Set("Sec-Fetch-Mode", "cors")
		h.Set("Sec-Fetch-Site", "same-origin")
		cookies := "ct0=" + csrf
		if s.token != "" {
			h.Set("X-Twitter-Auth-Type", "OAuth2Session")
			cookies = "auth_token=" + s.token + "; " + cookies
		} else {
			gt, err := s.activateGuest(ctx)
			if err != nil {
				return nil, 0, err
			}
			h.Set("X-Guest-Token", gt)
			cookies += "; gt=" + gt
		}
		h.Set("Cookie", cookies)
		if tid := s.transactionID(ctx, http.MethodGet, path); tid != "" {
			h.Set("X-Client-Transaction-Id", tid)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, xMaxResponse+1))
		resp.Body.Close()
		if err != nil {
			return nil, 0, err
		}
		if len(body) > xMaxResponse {
			return nil, 0, errors.New("X response exceeds the size limit")
		}
		for _, c := range resp.Cookies() {
			if c.Name == "ct0" && c.Value != "" {
				s.mu.Lock()
				s.csrf = c.Value
				s.mu.Unlock()
			}
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			reset := rateLimitReset(resp.Header)
			if wait := time.Until(reset); !reset.IsZero() && wait > 0 && wait <= time.Minute {
				select {
				case <-time.After(wait + time.Second):
					continue
				case <-ctx.Done():
					return nil, 0, ctx.Err()
				}
			}
			return nil, 0, &XRateLimitError{Reset: reset}
		}
		return body, resp.StatusCode, nil
	}
}

func rateLimitReset(h http.Header) time.Time {
	if v, err := strconv.ParseInt(h.Get("X-Rate-Limit-Reset"), 10, 64); err == nil && v > 0 {
		return time.Unix(v, 0)
	}
	if v, err := strconv.Atoi(h.Get("Retry-After")); err == nil && v > 0 {
		return time.Now().Add(time.Duration(v) * time.Second)
	}
	return time.Time{}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
