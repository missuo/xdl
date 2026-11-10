package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/settings"
)

func newTestServer(t *testing.T, addr string) *Server {
	t.Helper()
	eng := engine.New(settings.OpenPath(filepath.Join(t.TempDir(), "settings.json")))
	return New(eng, schedule.New(eng), addr, "test", "serve")
}

func call(h http.Handler, method, path, origin, token, host, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGuardLocal(t *testing.T) {
	s := newTestServer(t, "127.0.0.1:0")
	h := s.Handler()
	const host = "127.0.0.1:47333"
	job := `{"username":"nasa","mode":"fetch"}`
	for _, tc := range []struct {
		name, method, path, origin, token, host, body string
		want                                          int
	}{
		{"read-only GET needs no token", "GET", "/api/v1/jobs", "", "", host, "", 200},
		{"web pages are refused", "POST", "/api/v1/jobs", "https://evil.example", "", host, job, 403},
		{"web pages cannot read either", "GET", "/api/v1/jobs", "https://evil.example", "", host, "", 403},
		{"local programs need the token", "POST", "/api/v1/jobs", "", "", host, job, 401},
		{"with the token", "POST", "/api/v1/jobs", "", s.Token(), host, job, 201},
		{"extensions need no token", "POST", "/api/v1/jobs", "chrome-extension://abc", "", host, `{"username":"nasa2","mode":"fetch"}`, 201},
		{"a duplicate returns the job", "POST", "/api/v1/jobs", "chrome-extension://abc", "", host, `{"username":"nasa2","mode":"fetch"}`, 200},
		{"DNS rebinding is refused", "GET", "/api/v1/status", "", s.Token(), "evil.example:47333", "", 403},
		{"preflight", "OPTIONS", "/api/v1/jobs", "chrome-extension://abc", "", host, "", 204},
		{"legacy enqueue", "POST", "/enqueue", "chrome-extension://abc", "", host, `{"username":"@nasa3"}`, 200},
	} {
		if rec := call(h, tc.method, tc.path, tc.origin, tc.token, tc.host, tc.body); rec.Code != tc.want {
			t.Errorf("%s: %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
	rec := call(h, "GET", "/api/v1/jobs", "", "", host, "")
	var reply struct{ Jobs []engine.Job }
	_ = json.Unmarshal(rec.Body.Bytes(), &reply)
	if len(reply.Jobs) != 3 {
		t.Errorf("%d jobs queued, want 3", len(reply.Jobs))
	}
	if reply.Jobs[0].Source != "extension" {
		t.Errorf("legacy job source %q", reply.Jobs[0].Source)
	}
}

func TestGuardRemote(t *testing.T) {
	t.Setenv("XDL_API_TOKEN", "fixed")
	s := newTestServer(t, "0.0.0.0:0")
	h := s.Handler()
	if rec := call(h, "GET", "/api/v1/status", "", "", "nas.lan:47333", ""); rec.Code != 401 {
		t.Errorf("remote GET without token: %d", rec.Code)
	}
	if rec := call(h, "GET", "/api/v1/jobs", "", "fixed", "nas.lan:47333", ""); rec.Code != 200 {
		t.Errorf("remote GET with token: %d", rec.Code)
	}
	if rec := call(h, "POST", "/api/v1/jobs", "chrome-extension://abc", "", "nas.lan:47333", `{"username":"a"}`); rec.Code != 401 {
		t.Errorf("remote extension without token: %d", rec.Code)
	}
}

func TestMCP(t *testing.T) {
	s := newTestServer(t, "127.0.0.1:0")
	h := s.Handler()
	host := "127.0.0.1:47333"
	rec := call(h, "POST", "/mcp", "", s.Token(), host,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	if !strings.Contains(rec.Body.String(), `"protocolVersion":"2025-03-26"`) {
		t.Errorf("initialize: %s", rec.Body)
	}
	if rec := call(h, "POST", "/mcp", "", s.Token(), host, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); rec.Code != 202 {
		t.Errorf("notification: %d", rec.Code)
	}
	rec = call(h, "POST", "/mcp", "", s.Token(), host,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"download_account","arguments":{"username":"@nasa","mode":"fetch"}}}`)
	if !strings.Contains(rec.Body.String(), `\"source\": \"agent\"`) || strings.Contains(rec.Body.String(), `"isError":true`) {
		t.Errorf("download_account: %s", rec.Body)
	}
	rec = call(h, "POST", "/mcp", "", s.Token(), host,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"download_account","arguments":{}}}`)
	if !strings.Contains(rec.Body.String(), `"isError":true`) {
		t.Errorf("a bad call is not a tool error: %s", rec.Body)
	}
	if rec := call(h, "POST", "/mcp", "", "", host, `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`); rec.Code != 401 {
		t.Errorf("MCP without token: %d", rec.Code)
	}
}
