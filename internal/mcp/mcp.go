// Package mcp is XDL's Model Context Protocol server: tools through which
// an agent downloads accounts, follows jobs, searches the library and
// manages subscriptions. The API serves it over HTTP at /mcp, and `xdl mcp`
// over stdio for agents that start their servers as processes.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/schedule"
)

// ProtocolVersion is the newest MCP revision the server speaks.
const ProtocolVersion = "2025-06-18"

var supportedVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Server answers MCP requests for an engine and a scheduler.
type Server struct {
	Eng     *engine.Engine
	Sched   *schedule.Scheduler
	Version string
}

// Request is a JSON-RPC request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HandleMessage answers one JSON-RPC message, or returns nil for a
// notification.
func (s *Server) HandleMessage(ctx context.Context, data []byte) []byte {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		out, _ := json.Marshal(Response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &RPCError{-32700, "parse error"}})
		return out
	}
	if len(req.ID) == 0 {
		return nil // a notification, such as notifications/initialized
	}
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	result, err := s.handle(ctx, req)
	if err != nil {
		var rpc *RPCError
		if errors.As(err, &rpc) {
			resp.Error = rpc
		} else {
			resp.Error = &RPCError{-32603, err.Error()}
		}
	} else {
		resp.Result = result
	}
	out, _ := json.Marshal(resp)
	return out
}

func (e *RPCError) Error() string { return e.Message }

const instructions = `XDL downloads the media (photos, videos, GIFs, optionally text) of X/Twitter accounts to the user's disk.
Typical flow: download_account with a username or profile URL, then get_job (or pass wait_seconds) to see how it went.
Downloads are incremental and remembered: asking again for an account only fetches and downloads what is new.
subscribe makes XDL check an account on a schedule (daily by default) and download what it posts.`

func (s *Server) handle(ctx context.Context, req Request) (any, error) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := ProtocolVersion
		for _, v := range supportedVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "xdl", "title": "XDL", "version": s.Version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &RPCError{-32602, "invalid params"}
		}
		t, ok := tools[p.Name]
		if !ok {
			return nil, &RPCError{-32602, "unknown tool " + p.Name}
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		out, err := t.run(ctx, s, p.Arguments)
		if err != nil {
			return toolResult(map[string]string{"error": err.Error()}, true), nil
		}
		return toolResult(out, false), nil
	}
	return nil, &RPCError{-32601, "method not found: " + req.Method}
}

func toolResult(v any, isError bool) map[string]any {
	data, _ := json.MarshalIndent(v, "", "  ")
	res := map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(data)}},
		"isError": isError,
	}
	if !isError {
		if _, ok := v.(map[string]any); ok {
			res["structuredContent"] = v
		}
	}
	return res
}

type tool struct {
	def map[string]any
	run func(ctx context.Context, s *Server, args json.RawMessage) (any, error)
}

var (
	tools    = map[string]tool{}
	toolList []map[string]any
)

func register(name, title, description string, props map[string]any, required []string, readOnly bool,
	run func(ctx context.Context, s *Server, args json.RawMessage) (any, error)) {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	def := map[string]any{
		"name": name, "title": title, "description": description, "inputSchema": schema,
		"annotations": map[string]any{"readOnlyHint": readOnly, "openWorldHint": true},
	}
	tools[name] = tool{def: def, run: run}
	toolList = append(toolList, def)
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

var timelineProp = map[string]any{
	"type": "string", "enum": engine.TimelineTypes,
	"description": "Which timeline: media (default: the account's photos, videos and GIFs), timeline, tweets, with_replies, likes, or bookmarks (the signed-in user's).",
}

func init() {
	register("xdl_status", "XDL status",
		"Whether XDL is ready (auth token saved, xtractor installed), where it downloads to, and what it is doing.",
		map[string]any{}, nil, true,
		func(ctx context.Context, s *Server, _ json.RawMessage) (any, error) {
			queued, running := s.Eng.Counts()
			token, _ := backend.GetStoredAuthToken("public")
			dir := s.Eng.Settings().Get().DownloadDir()
			out := map[string]any{
				"version": s.Version, "queued": queued, "running": running,
				"auth_token_saved": token != "", "xtractor_installed": backend.IsExtractorInstalled(),
				"download_dir": dir,
			}
			if j, ok := s.Eng.Active(); ok {
				out["active_job"] = j
			}
			return out, nil
		})

	register("download_account", "Download an X account's media",
		"Queue a job that fetches an X/Twitter account's timeline and downloads its media to the user's disk. "+
			"Incremental: only what is new since the last time is fetched, and media downloaded before is skipped without a request. "+
			"Returns the job; pass wait_seconds to wait for it to finish (or poll get_job).",
		map[string]any{
			"username":      str("The @handle (with or without @) or a profile URL such as https://x.com/nasa. Not needed for bookmarks."),
			"timeline_type": timelineProp,
			"mode": map[string]any{"type": "string", "enum": []string{"download", "fetch", "library"},
				"description": "download (default): fetch and download. fetch: only save the timeline to XDL's library. library: download what the library already holds, fetching nothing."},
			"full":           boolean("Fetch the whole timeline again instead of stopping where the library's copy begins."),
			"redownload":     boolean("Ignore the download history (files still on disk are skipped)."),
			"include_photos": boolean("Override the setting: include photos."),
			"include_videos": boolean("Override the setting: include videos."),
			"include_gifs":   boolean("Override the setting: include GIFs."),
			"include_text":   boolean("Override the setting: include text-only tweets."),
			"retweets":       boolean("Override the setting: include retweets."),
			"wait_seconds":   integer("Wait up to this many seconds (at most 600) for the job to finish before returning."),
		}, nil, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				engine.Request
				Wait int `json:"wait_seconds"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			a.Source = "agent"
			j, err := s.Eng.Submit(a.Request)
			dup := errors.Is(err, engine.ErrDuplicate)
			if err != nil && !dup {
				return nil, err
			}
			j = wait(ctx, s.Eng, j, a.Wait)
			return map[string]any{"job": j, "already_queued": dup, "summary": j.Label() + ": " + j.Summary()}, nil
		})

	register("get_job", "Get a job",
		"A job's state and counts; include_items lists its media items with their download status and file paths.",
		map[string]any{
			"id":            str("The job id."),
			"include_items": boolean("Also list the job's media items (can be long)."),
			"wait_seconds":  integer("Wait up to this many seconds (at most 600) for the job to finish first."),
		}, []string{"id"}, true,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				ID    string `json:"id"`
				Items bool   `json:"include_items"`
				Wait  int    `json:"wait_seconds"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			j, ok := s.Eng.Get(a.ID)
			if !ok {
				return nil, fmt.Errorf("no job %q", a.ID)
			}
			j = wait(ctx, s.Eng, j, a.Wait)
			out := map[string]any{"job": j, "summary": j.Label() + ": " + j.Summary()}
			if a.Items {
				items, _ := s.Eng.Items(a.ID)
				out["items"] = items
			}
			return out, nil
		})

	register("list_jobs", "List jobs", "XDL's queued, running and recent jobs, newest first.",
		map[string]any{"limit": integer("At most this many (default 20).")}, nil, true,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				Limit int `json:"limit"`
			}
			_ = json.Unmarshal(args, &a)
			if a.Limit <= 0 {
				a.Limit = 20
			}
			jobs := s.Eng.Jobs()
			if len(jobs) > a.Limit {
				jobs = jobs[:a.Limit]
			}
			return map[string]any{"jobs": jobs}, nil
		})

	register("cancel_job", "Cancel a job", "Stop a queued or running job. Files already downloaded stay.",
		map[string]any{"id": str("The job id.")}, []string{"id"}, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(args, &a)
			return map[string]any{"cancelled": s.Eng.Cancel(a.ID)}, nil
		})

	register("search_library", "Search the library",
		"Accounts XDL has fetched before, with their item counts and when they were fetched.",
		map[string]any{"query": str("Filter by handle or name (optional).")}, nil, true,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(args, &a)
			accounts, err := backend.GetAllAccounts()
			if err != nil {
				return nil, err
			}
			q := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a.Query), "@"))
			out := make([]backend.AccountListItem, 0, len(accounts))
			for _, acc := range accounts {
				if q == "" || strings.Contains(strings.ToLower(acc.Username), q) || strings.Contains(strings.ToLower(acc.Name), q) {
					out = append(out, acc)
				}
			}
			return map[string]any{"accounts": out}, nil
		})

	register("list_subscriptions", "List subscriptions",
		"Accounts XDL checks on a schedule, with their interval, last check and next check.",
		map[string]any{}, nil, true,
		func(ctx context.Context, s *Server, _ json.RawMessage) (any, error) {
			subs, err := backend.ListSubscriptions()
			if err != nil {
				return nil, err
			}
			return map[string]any{"subscriptions": subs}, nil
		})

	register("subscribe", "Subscribe to an account",
		"Make XDL check an account on a schedule and download what it posts since. The first check starts at once. "+
			"Subscribing again to the same account and timeline changes its interval.",
		map[string]any{
			"username":      str("The @handle or profile URL."),
			"timeline_type": timelineProp,
			"interval":      str("How often: 6h, 12h, 1d (default), 1w … at least 15m."),
		}, []string{"username"}, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				Username     string `json:"username"`
				TimelineType string `json:"timeline_type"`
				Interval     string `json:"interval"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			d, err := schedule.ParseInterval(a.Interval)
			if err != nil {
				return nil, err
			}
			sub, err := s.Sched.Subscribe(a.Username, a.TimelineType, d)
			if err != nil {
				return nil, err
			}
			return map[string]any{"subscription": sub}, nil
		})

	register("update_subscription", "Change a subscription", "Change a subscription's interval, or pause and resume it.",
		map[string]any{
			"id":       integer("The subscription id."),
			"interval": str("New interval, as 1d (optional)."),
			"enabled":  boolean("false pauses the subscription, true resumes it."),
		}, []string{"id", "enabled"}, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				ID       int64  `json:"id"`
				Interval string `json:"interval"`
				Enabled  bool   `json:"enabled"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			var d time.Duration
			if a.Interval != "" {
				var err error
				if d, err = schedule.ParseInterval(a.Interval); err != nil {
					return nil, err
				}
			}
			sub, err := s.Sched.Update(a.ID, d, a.Enabled)
			if err != nil {
				return nil, err
			}
			return map[string]any{"subscription": sub}, nil
		})

	register("unsubscribe", "Unsubscribe", "Stop checking an account. Its downloads and library entry stay.",
		map[string]any{"id": integer("The subscription id.")}, []string{"id"}, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(args, &a)
			if err := s.Sched.Unsubscribe(a.ID); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		})

	register("check_subscription_now", "Check a subscription now",
		"Check a subscribed account at once instead of waiting for its next scheduled check.",
		map[string]any{"id": integer("The subscription id.")}, []string{"id"}, false,
		func(ctx context.Context, s *Server, args json.RawMessage) (any, error) {
			var a struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(args, &a)
			j, err := s.Sched.RunNow(a.ID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"job": j}, nil
		})
}

// wait follows a job until it finishes, seconds pass or ctx is done.
func wait(ctx context.Context, eng *engine.Engine, j engine.Job, seconds int) engine.Job {
	if seconds <= 0 || j.State.Finished() {
		return j
	}
	seconds = min(seconds, 600)
	events, unsubscribe := eng.Subscribe()
	defer unsubscribe()
	// It may have finished before the subscription.
	if cur, ok := eng.Get(j.ID); ok {
		j = cur
		if j.State.Finished() {
			return j
		}
	}
	deadline := time.NewTimer(time.Duration(seconds) * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return j
		case <-deadline.C:
			return j
		case ev := <-events:
			if ev.Job != nil && ev.Job.ID == j.ID {
				j = *ev.Job
				if j.State.Finished() {
					return j
				}
			}
		}
	}
}
