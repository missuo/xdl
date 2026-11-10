package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/client"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/mcp"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/server"
	"github.com/missuo/xdl/internal/settings"
	"github.com/missuo/xdl/internal/version"
)

// cmdSub manages subscriptions through the running XDL, which schedules
// them. Without one they are saved for the next XDL that runs.
func cmdSub(args []string) error {
	sub := "ls"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	c, connErr := client.Connect()
	ctx := context.Background()
	local := func() error {
		if connErr == nil {
			return nil
		}
		return initDB()
	}
	switch sub {
	case "ls", "list":
		fs := flag.NewFlagSet("sub ls", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "print JSON")
		if _, err := parse(fs, args); err != nil {
			return err
		}
		var subs []backend.Subscription
		var err error
		if c != nil {
			subs, err = c.Subscriptions(ctx)
		} else if err = local(); err == nil {
			subs, err = backend.ListSubscriptions()
		}
		if err != nil {
			return err
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(subs)
		}
		if len(subs) == 0 {
			fmt.Println("No subscriptions. Add one with `xdl sub add <user> [interval]`.")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tACCOUNT\tEVERY\tLAST CHECK\tRESULT\tNEXT CHECK")
		for _, s := range subs {
			last := "never"
			if s.LastRun != nil {
				last = s.LastRun.Local().Format(time.DateTime)
			}
			next := s.NextRun.Local().Format(time.DateTime)
			if !s.Enabled {
				next = "paused"
			}
			label := engine.Job{Username: s.Username, TimelineType: s.TimelineType}.Label()
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", s.ID, label, schedule.Describe(s.IntervalMins), last,
				truncate(cmpOr(s.LastMessage, s.LastStatus), 44), next)
		}
		if c == nil {
			fmt.Fprintln(tw, "\n(XDL is not running: subscriptions are checked while the app or `xdl serve` runs)")
		}
		return tw.Flush()

	case "add":
		fs := flag.NewFlagSet("sub add", flag.ContinueOnError)
		timeline := fs.String("type", "media", "timeline: media, timeline, tweets, with_replies, likes, bookmarks")
		every := fs.String("every", "", "how often: 6h, 12h, 1d (default), 1w")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) == 0 {
			return errors.New("usage: xdl sub add <user> [interval] [-type media]")
		}
		if len(pos) > 1 && *every == "" {
			*every = pos[1]
		}
		d, err := schedule.ParseInterval(*every)
		if err != nil {
			return err
		}
		var s backend.Subscription
		if c != nil {
			s, err = c.Subscribe(ctx, server.SubscriptionRequest{Username: pos[0], TimelineType: *timeline, Interval: *every})
		} else if err = local(); err == nil {
			s, err = schedule.New(engine.New(settings.Open())).Subscribe(pos[0], *timeline, d)
		}
		if err != nil {
			return err
		}
		label := engine.Job{Username: s.Username, TimelineType: s.TimelineType}.Label()
		if c != nil {
			fmt.Printf("Subscribed to %s (id %d): checking now, then every %s.\n", label, s.ID, schedule.Describe(s.IntervalMins))
		} else {
			fmt.Printf("Subscribed to %s (id %d), every %s. It is checked once XDL or `xdl serve` runs.\n", label, s.ID, schedule.Describe(s.IntervalMins))
		}
		return nil

	case "rm", "remove", "pause", "resume", "run":
		if len(args) == 0 {
			return fmt.Errorf("usage: xdl sub %s <id>…", sub)
		}
		for _, arg := range args {
			id, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", arg)
			}
			if err := subAction(ctx, c, local, sub, id); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown sub command %q (ls, add, rm, pause, resume, run)", sub)
}

func subAction(ctx context.Context, c *client.Client, local func() error, action string, id int64) error {
	enabled := action == "resume"
	if c != nil {
		switch action {
		case "rm", "remove":
			if err := c.Unsubscribe(ctx, id); err != nil {
				return err
			}
			fmt.Println("Unsubscribed", id)
		case "pause", "resume":
			if _, err := c.UpdateSubscription(ctx, id, server.SubscriptionRequest{Enabled: &enabled}); err != nil {
				return err
			}
			fmt.Printf("%s %d\n", map[bool]string{true: "Resumed", false: "Paused"}[enabled], id)
		case "run":
			j, err := c.RunSubscription(ctx, id)
			if err != nil {
				return err
			}
			fmt.Printf("Checking %s (job %s) — follow it with `xdl watch %s`\n", j.Label(), j.ID, j.ID)
		}
		return nil
	}
	if err := local(); err != nil {
		return err
	}
	switch action {
	case "rm", "remove":
		if err := backend.DeleteSubscription(id); err != nil {
			return err
		}
		fmt.Println("Unsubscribed", id)
	case "pause", "resume":
		if _, err := backend.UpdateSubscription(id, 0, enabled); err != nil {
			return err
		}
		fmt.Printf("%s %d\n", map[bool]string{true: "Resumed", false: "Paused"}[enabled], id)
	case "run":
		return client.ErrNotRunning
	}
	return nil
}

// cmdMCP serves MCP over stdio: through the running XDL when there is one,
// so the agent's jobs join its queue, or in this process otherwise.
func cmdMCP(args []string) error {
	in := bufio.NewReader(os.Stdin)
	var mu sync.Mutex
	write := func(msg []byte) {
		mu.Lock()
		defer mu.Unlock()
		os.Stdout.Write(append(msg, '\n'))
	}
	var handle func(ctx context.Context, msg []byte) []byte
	if c, err := client.Connect(); err == nil {
		handle = func(ctx context.Context, msg []byte) []byte {
			out, err := c.MCP(ctx, msg)
			if err != nil {
				return rpcError(msg, err)
			}
			return out
		}
	} else {
		if err := initDB(); err != nil {
			return err
		}
		eng := engine.New(settings.Open())
		go eng.Run(context.Background())
		srv := &mcp.Server{Eng: eng, Sched: schedule.New(eng), Version: version.Version}
		handle = srv.HandleMessage
	}
	var wg sync.WaitGroup
	for {
		line, err := in.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			msg := append([]byte(nil), line...)
			wg.Go(func() {
				if out := handle(context.Background(), msg); out != nil {
					write(out)
				}
			})
		}
		if err == io.EOF {
			wg.Wait()
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func rpcError(msg []byte, err error) []byte {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(msg, &req)
	if len(req.ID) == 0 {
		return nil
	}
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": req.ID,
		"error": map[string]any{"code": -32603, "message": err.Error()},
	})
	return out
}
