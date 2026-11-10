// Package server is XDL's local HTTP API on 127.0.0.1:47333. The browser
// extension submits jobs through it, and the CLI drives a running app or
// `xdl serve` with it.
//
// Who may call it:
//   - browser extensions: requests whose Origin is chrome-extension://,
//     moz-extension:// or safari-web-extension:// (web pages cannot forge
//     the Origin header);
//   - local programs: requests without an Origin, with the bearer token
//     the server writes to api.json (0600) in XDL's data directory; read-only
//     GETs need no token;
//   - nothing else: any other Origin is refused, and so is a Host header
//     other than the loopback address, which defeats DNS rebinding.
//
// Listening on another address (`xdl serve -addr 0.0.0.0:47333`, in Docker
// say) is remote mode: every request then needs the bearer token, which
// XDL_API_TOKEN fixes so that clients elsewhere can know it.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/mcp"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/settings"
)

// DefaultAddr is where the extension expects XDL.
const DefaultAddr = "127.0.0.1:47333"

// Info is what the server writes to api.json for local clients.
type Info struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
	Kind    string `json:"kind"` // gui or serve
}

// InfoPath is the api.json file.
func InfoPath() string { return filepath.Join(settings.Dir(), "api.json") }

// ReadInfo reads api.json.
func ReadInfo() (Info, error) {
	var info Info
	data, err := os.ReadFile(InfoPath())
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return info, err
}

// Server serves the API for an engine.
type Server struct {
	eng     *engine.Engine
	sched   *schedule.Scheduler
	mcp     *mcp.Server
	addr    string
	token   string
	version string
	kind    string
	remote  bool
	ln      net.Listener
}

// New returns a server for eng. kind names the process (gui or serve).
func New(eng *engine.Engine, sched *schedule.Scheduler, addr, version, kind string) *Server {
	if addr == "" {
		addr = os.Getenv("XDL_API_ADDR") // for running a second copy while testing
	}
	if addr == "" {
		addr = DefaultAddr
	}
	token := os.Getenv("XDL_API_TOKEN")
	if token == "" {
		var b [24]byte
		_, _ = rand.Read(b[:])
		token = hex.EncodeToString(b[:])
	}
	host, _, _ := net.SplitHostPort(addr)
	return &Server{
		eng: eng, sched: sched, addr: addr, token: token, version: version, kind: kind,
		remote: !loopbackHost(host),
		mcp:    &mcp.Server{Eng: eng, Sched: sched, Version: version},
	}
}

// Token is the bearer token clients must send.
func (s *Server) Token() string { return s.token }

// Addr is the address the server listens on.
func (s *Server) Addr() string { return s.addr }

// ErrInUse means another process already serves the API.
var ErrInUse = errors.New("the XDL API port is already in use (is XDL or `xdl serve` running?)")

// Listen takes the address. Call Serve next.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return ErrInUse
		}
		return err
	}
	s.ln = ln
	return nil
}

// Run listens and serves until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	if err := s.Listen(); err != nil {
		return err
	}
	return s.Serve(ctx)
}

// Serve serves on the address Listen took until ctx is done.
func (s *Server) Serve(ctx context.Context) error {
	ln := s.ln
	info := Info{Address: s.addr, Token: s.token, PID: os.Getpid(), Version: s.version, Kind: s.kind}
	if s.remote {
		// Local clients reach a wildcard address on the loopback.
		if _, port, err := net.SplitHostPort(s.addr); err == nil {
			info.Address = net.JoinHostPort("127.0.0.1", port)
		}
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	_ = os.MkdirAll(settings.Dir(), 0o755)
	if err := os.WriteFile(InfoPath(), data, 0o600); err != nil {
		log.Printf("api: writing %s: %v", InfoPath(), err)
	}
	defer func() {
		// Only remove our own file: a second instance may have replaced it.
		if cur, err := ReadInfo(); err == nil && cur.PID == os.Getpid() {
			_ = os.Remove(InfoPath())
		}
	}()

	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("api: listening on http://%s", s.addr)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Handler is the API's handler, for tests.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/jobs", s.listJobs)
	mux.HandleFunc("POST /api/v1/jobs", s.createJob)
	mux.HandleFunc("POST /api/v1/jobs/clear", s.clearJobs)
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.getJob)
	mux.HandleFunc("DELETE /api/v1/jobs/{id}", s.deleteJob)
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.cancelJob)
	mux.HandleFunc("GET /api/v1/jobs/{id}/items", s.jobItems)
	mux.HandleFunc("GET /api/v1/events", s.events)
	mux.HandleFunc("GET /api/v1/library", s.library)
	mux.HandleFunc("GET /api/v1/subscriptions", s.listSubscriptions)
	mux.HandleFunc("POST /api/v1/subscriptions", s.createSubscription)
	mux.HandleFunc("PATCH /api/v1/subscriptions/{id}", s.updateSubscription)
	mux.HandleFunc("DELETE /api/v1/subscriptions/{id}", s.deleteSubscription)
	mux.HandleFunc("POST /api/v1/subscriptions/{id}/run", s.runSubscription)
	mux.HandleFunc("POST /mcp", s.serveMCP)
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, errors.New("POST JSON-RPC to /mcp"))
	})

	// The 1.x extension's endpoints.
	mux.HandleFunc("POST /enqueue", s.legacyEnqueue)
	mux.HandleFunc("GET /status", s.legacyStatus)
	return s.guard(mux)
}

func trustedOrigin(origin string) bool {
	return strings.HasPrefix(origin, "chrome-extension://") ||
		strings.HasPrefix(origin, "moz-extension://") ||
		strings.HasPrefix(origin, "safari-web-extension://")
}

func loopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.remote && !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, errors.New("forbidden host"))
			return
		}
		origin := r.Header.Get("Origin")
		extension := origin != "" && trustedOrigin(origin)
		if extension {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else if origin != "" {
			writeError(w, http.StatusForbidden, errors.New("forbidden origin"))
			return
		}
		readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
		if s.remote || (!extension && !readOnly) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, errors.New("missing or wrong bearer token (see api.json, or XDL_API_TOKEN)"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Status is the reply of GET /api/v1/status.
type Status struct {
	App          string      `json:"app"`
	Version      string      `json:"version"`
	Kind         string      `json:"kind"`
	PID          int         `json:"pid"`
	Queued       int         `json:"queued"`
	Running      int         `json:"running"`
	Active       *engine.Job `json:"active,omitempty"`
	AuthToken    bool        `json:"auth_token"`
	DownloadPath string      `json:"download_path"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	queued, running := s.eng.Counts()
	st := Status{
		App: "XDL", Version: s.version, Kind: s.kind, PID: os.Getpid(),
		Queued: queued, Running: running,
	}
	if j, ok := s.eng.Active(); ok {
		st.Active = &j
	}
	if token, err := backend.GetStoredAuthToken("public"); err == nil && token != "" {
		st.AuthToken = true
	}
	st.DownloadPath = s.eng.Settings().Get().DownloadDir()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.eng.Jobs()})
}

// CreateReply is the reply of POST /api/v1/jobs.
type CreateReply struct {
	Job       engine.Job `json:"job"`
	Duplicate bool       `json:"duplicate"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req engine.Request
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Source == "" {
		req.Source = "api"
		if trustedOrigin(r.Header.Get("Origin")) {
			req.Source = "extension"
		}
	}
	s.submit(w, req)
}

func (s *Server) submit(w http.ResponseWriter, req engine.Request) {
	j, err := s.eng.Submit(req)
	switch {
	case errors.Is(err, engine.ErrDuplicate):
		writeJSON(w, http.StatusOK, CreateReply{Job: j, Duplicate: true})
	case err != nil:
		writeError(w, http.StatusBadRequest, err)
	default:
		writeJSON(w, http.StatusCreated, CreateReply{Job: j})
	}
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.eng.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.eng.Get(id); !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	if !s.eng.Remove(id) {
		writeError(w, http.StatusConflict, errors.New("the job has not finished; cancel it first"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.eng.Get(id); !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": s.eng.Cancel(id)})
}

func (s *Server) clearJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int{"removed": s.eng.ClearFinished()})
}

func (s *Server) jobItems(w http.ResponseWriter, r *http.Request) {
	items, ok := s.eng.Items(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) library(w http.ResponseWriter, r *http.Request) {
	accounts, err := backend.GetAllAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

// events streams engine events as server-sent events. Item events, which
// can number in the thousands per job, come only with ?items=1; ?job=ID
// keeps one job's events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	withItems := r.URL.Query().Get("items") == "1"
	only := r.URL.Query().Get("job")
	ch, unsubscribe := s.eng.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Type == engine.EventItem && !withItems {
				continue
			}
			if only != "" {
				if (ev.Job == nil || ev.Job.ID != only) && (ev.Item == nil || ev.Item.JobID != only) {
					continue
				}
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}

func (s *Server) legacyEnqueue(w http.ResponseWriter, r *http.Request) {
	var req engine.Request
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Source = "extension"
	j, err := s.eng.Submit(req)
	switch {
	case errors.Is(err, engine.ErrDuplicate):
		writeJSON(w, http.StatusOK, map[string]any{"queued": true, "duplicate": true, "id": j.ID})
	case err != nil:
		writeError(w, http.StatusBadRequest, err)
	default:
		queued, running := s.eng.Counts()
		writeJSON(w, http.StatusOK, map[string]any{"queued": true, "position": queued + running, "id": j.ID})
	}
}

func (s *Server) legacyStatus(w http.ResponseWriter, r *http.Request) {
	var queue []string
	for _, j := range s.eng.Jobs() {
		if !j.State.Finished() {
			queue = append(queue, j.Username)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": "XDL", "version": s.version, "queue": queue})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := backend.ListSubscriptions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if subs == nil {
		subs = []backend.Subscription{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}

// SubscriptionRequest is the body of POST and PATCH /api/v1/subscriptions.
type SubscriptionRequest struct {
	Username     string `json:"username,omitempty"`
	TimelineType string `json:"timeline_type,omitempty"`
	Interval     string `json:"interval,omitempty"` // 6h, 1d, 1w
	Enabled      *bool  `json:"enabled,omitempty"`
}

func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var req SubscriptionRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d, err := schedule.ParseInterval(req.Interval)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sub, err := s.sched.Subscribe(req.Username, req.TimelineType, d)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"subscription": sub})
}

func subscriptionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid subscription id"))
		return 0, false
	}
	return id, true
}

func (s *Server) updateSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	var req SubscriptionRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cur, err := backend.GetSubscription(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var d time.Duration
	if req.Interval != "" {
		if d, err = schedule.ParseInterval(req.Interval); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	enabled := cur.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sub, err := s.sched.Update(id, d, enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub})
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	if err := s.sched.Unsubscribe(id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) runSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	j, err := s.sched.RunNow(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": j})
}

// serveMCP is MCP's streamable HTTP transport, answering each POST with
// JSON.
func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out := s.mcp.HandleMessage(r.Context(), body)
	if out == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
