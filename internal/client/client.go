// Package client talks to a running XDL (the app or `xdl serve`) over its
// local API.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/server"
)

// Client is a connection to a running XDL.
type Client struct {
	base  string
	token string
	http  *http.Client
	Info  server.Info
}

var errUnauthorized = errors.New("unauthorized")

// ErrNotRunning means no XDL answers on the API port.
var ErrNotRunning = errors.New("XDL is not running (start the app, or run `xdl serve`)")

// Connect finds a running XDL and checks that it answers: XDL_API_URL and
// XDL_API_TOKEN name one elsewhere (`xdl serve` in Docker, say), and
// api.json the local one.
func Connect() (*Client, error) {
	info, err := server.ReadInfo()
	if err != nil {
		info = server.Info{Address: server.DefaultAddr}
	}
	base := "http://" + info.Address
	if u := strings.TrimRight(os.Getenv("XDL_API_URL"), "/"); u != "" {
		base, info = u, server.Info{Address: strings.TrimPrefix(strings.TrimPrefix(u, "http://"), "https://"), Kind: "serve"}
	}
	if t := os.Getenv("XDL_API_TOKEN"); t != "" {
		info.Token = t
	}
	c := &Client{
		base:  base,
		token: info.Token,
		http:  &http.Client{Timeout: 30 * time.Second},
		Info:  info,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Status(ctx); err != nil {
		if errors.Is(err, errUnauthorized) {
			return nil, errors.New("XDL refused the API token (check XDL_API_TOKEN)")
		}
		return nil, ErrNotRunning
	}
	return c, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Status returns the server's status.
func (c *Client) Status(ctx context.Context) (server.Status, error) {
	var st server.Status
	err := c.do(ctx, http.MethodGet, "/api/v1/status", nil, &st)
	return st, err
}

// Submit queues a job.
func (c *Client) Submit(ctx context.Context, req engine.Request) (server.CreateReply, error) {
	var reply server.CreateReply
	err := c.do(ctx, http.MethodPost, "/api/v1/jobs", req, &reply)
	return reply, err
}

// Jobs lists the jobs, newest first.
func (c *Client) Jobs(ctx context.Context) ([]engine.Job, error) {
	var reply struct {
		Jobs []engine.Job `json:"jobs"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/jobs", nil, &reply)
	return reply.Jobs, err
}

// Job returns one job.
func (c *Client) Job(ctx context.Context, id string) (engine.Job, error) {
	var j engine.Job
	err := c.do(ctx, http.MethodGet, "/api/v1/jobs/"+id, nil, &j)
	return j, err
}

// Cancel stops a job.
func (c *Client) Cancel(ctx context.Context, id string) (bool, error) {
	var reply struct {
		Cancelled bool `json:"cancelled"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/jobs/"+id+"/cancel", struct{}{}, &reply)
	return reply.Cancelled, err
}

// Clear forgets finished jobs.
func (c *Client) Clear(ctx context.Context) (int, error) {
	var reply struct {
		Removed int `json:"removed"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/jobs/clear", struct{}{}, &reply)
	return reply.Removed, err
}

// Events streams job events until ctx is done or fn returns false. A
// non-empty jobID keeps that job's events alone.
func (c *Client) Events(ctx context.Context, jobID string, fn func(engine.Event) bool) error {
	path := "/api/v1/events"
	if jobID != "" {
		path += "?job=" + jobID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	stream := &http.Client{} // no timeout: the stream lasts
	resp, err := stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("events: %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev engine.Event
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		if !fn(ev) {
			return nil
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return sc.Err()
}

// Subscriptions lists the subscriptions.
func (c *Client) Subscriptions(ctx context.Context) ([]backend.Subscription, error) {
	var reply struct {
		Subscriptions []backend.Subscription `json:"subscriptions"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/subscriptions", nil, &reply)
	return reply.Subscriptions, err
}

// Subscribe adds or changes a subscription.
func (c *Client) Subscribe(ctx context.Context, req server.SubscriptionRequest) (backend.Subscription, error) {
	var reply struct {
		Subscription backend.Subscription `json:"subscription"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/subscriptions", req, &reply)
	return reply.Subscription, err
}

// UpdateSubscription changes a subscription's interval or pauses it.
func (c *Client) UpdateSubscription(ctx context.Context, id int64, req server.SubscriptionRequest) (backend.Subscription, error) {
	var reply struct {
		Subscription backend.Subscription `json:"subscription"`
	}
	err := c.do(ctx, http.MethodPatch, "/api/v1/subscriptions/"+strconv.FormatInt(id, 10), req, &reply)
	return reply.Subscription, err
}

// Unsubscribe removes a subscription.
func (c *Client) Unsubscribe(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/subscriptions/"+strconv.FormatInt(id, 10), nil, nil)
}

// RunSubscription checks a subscription now.
func (c *Client) RunSubscription(ctx context.Context, id int64) (engine.Job, error) {
	var reply struct {
		Job engine.Job `json:"job"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/subscriptions/"+strconv.FormatInt(id, 10)+"/run", struct{}{}, &reply)
	return reply.Job, err
}

// MCP sends one MCP JSON-RPC message to the server's /mcp and returns its
// reply, or nil for a notification.
func (c *Client) MCP(ctx context.Context, message []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/mcp", bytes.NewReader(message))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := (&http.Client{Timeout: 11 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp: %s: %s", resp.Status, bytes.TrimSpace(data))
	}
	return data, nil
}
