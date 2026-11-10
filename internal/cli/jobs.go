package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/missuo/xdl/internal/client"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/settings"
)

// optBool is a flag that records whether it was given, so only the flags
// the user typed override the settings.
type optBool struct{ v *bool }

func (o *optBool) String() string {
	if o == nil || o.v == nil {
		return ""
	}
	return fmt.Sprint(*o.v)
}

func (o *optBool) Set(s string) error {
	b := s == "" || s == "true" || s == "1" || s == "yes"
	if !b && s != "false" && s != "0" && s != "no" {
		return fmt.Errorf("expected true or false")
	}
	o.v = &b
	return nil
}

func (o *optBool) IsBoolFlag() bool { return true }

type jobFlags struct {
	fs                                   *flag.FlagSet
	timeline                             string
	full, local, detach, fromLibrary     bool
	redownload                           bool
	out                                  string
	photos, videos, gifs, text, retweets optBool
}

func newJobFlags(name string) *jobFlags {
	f := &jobFlags{fs: flag.NewFlagSet(name, flag.ContinueOnError)}
	f.fs.StringVar(&f.timeline, "type", "media", "timeline: "+strings.Join(engine.TimelineTypes, ", "))
	f.fs.BoolVar(&f.full, "full", false, "refetch the whole timeline, not only what is new")
	f.fs.BoolVar(&f.redownload, "redownload", false, "ignore the download history (files on disk are still skipped)")
	f.fs.BoolVar(&f.local, "local", false, "run in this process even when XDL is running")
	f.fs.BoolVar(&f.detach, "detach", false, "queue the job in the running XDL and return")
	f.fs.StringVar(&f.out, "out", "", "download directory (default: the setting)")
	f.fs.Var(&f.photos, "photos", "include photos (default: the setting)")
	f.fs.Var(&f.videos, "videos", "include videos")
	f.fs.Var(&f.gifs, "gifs", "include GIFs")
	f.fs.Var(&f.text, "text", "include text-only tweets")
	f.fs.Var(&f.retweets, "retweets", "include retweets")
	return f
}

func (f *jobFlags) request(user string, mode engine.Mode) engine.Request {
	return engine.Request{
		Username:      user,
		TimelineType:  f.timeline,
		Mode:          mode,
		Full:          f.full,
		Redownload:    f.redownload,
		Source:        "cli",
		OutputDir:     f.out,
		IncludePhotos: f.photos.v,
		IncludeVideos: f.videos.v,
		IncludeGifs:   f.gifs.v,
		IncludeText:   f.text.v,
		Retweets:      f.retweets.v,
	}
}

func cmdDownload(args []string) error { return jobCommand("download", args, engine.ModeDownload) }
func cmdFetch(args []string) error    { return jobCommand("fetch", args, engine.ModeFetch) }

func cmdSend(args []string) error {
	return jobCommand("send", append([]string{"-detach"}, args...), engine.ModeDownload)
}

func jobCommand(name string, args []string, mode engine.Mode) error {
	f := newJobFlags(name)
	if mode == engine.ModeDownload {
		f.fs.BoolVar(&f.fromLibrary, "from-library", false, "download what the library holds, fetching nothing")
	}
	f.fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: xdl %s <user|url> [flags]\n\n", name)
		f.fs.PrintDefaults()
	}
	pos, err := parse(f.fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 && f.timeline != "bookmarks" {
		f.fs.Usage()
		return errors.New("a username is required")
	}
	if f.fromLibrary {
		mode = engine.ModeLibrary
	}
	users := pos
	if len(users) == 0 {
		users = []string{""}
	}
	if f.local && f.detach {
		return errors.New("-local and -detach exclude each other")
	}

	var c *client.Client
	if !f.local {
		c, err = client.Connect()
		if err != nil && f.detach {
			return err
		}
	}
	for _, user := range users {
		req := f.request(user, mode)
		if c != nil {
			err = runRemote(c, req, f.detach)
		} else {
			err = runLocal(req)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func runRemote(c *client.Client, req engine.Request, detach bool) error {
	ctx, cancel := interruptible()
	defer cancel()
	reply, err := c.Submit(ctx, req)
	if err != nil {
		return err
	}
	j := reply.Job
	where := "XDL"
	if c.Info.Kind == "serve" {
		where = "xdl serve"
	}
	if reply.Duplicate {
		fmt.Printf("%s is already queued in %s (job %s)\n", j.Label(), where, j.ID)
	} else {
		fmt.Printf("Queued %s in %s (job %s)\n", j.Label(), where, j.ID)
	}
	if detach {
		return nil
	}
	p := newProgress()
	var final engine.Job
	err = c.Events(ctx, j.ID, func(ev engine.Event) bool {
		if ev.Job == nil || ev.Removed {
			return true
		}
		p.show(*ev.Job)
		if ev.Job.State.Finished() {
			final = *ev.Job
			return false
		}
		return true
	})
	if ctx.Err() != nil {
		p.done()
		fmt.Printf("Detached; the job goes on in %s. Stop it with `xdl cancel %s`.\n", where, j.ID)
		return nil
	}
	if err != nil {
		return err
	}
	if final.ID == "" {
		// The stream ended before the job did; ask once more.
		if final, err = c.Job(context.Background(), j.ID); err != nil {
			return err
		}
	}
	return p.finish(final)
}

func runLocal(req engine.Request) error {
	if err := initDB(); err != nil {
		return err
	}
	eng := engine.New(settings.Open())
	events, unsubscribe := eng.Subscribe()
	defer unsubscribe()
	runCtx, stopEngine := context.WithCancel(context.Background())
	defer stopEngine()
	go eng.Run(runCtx)

	j, err := eng.Submit(req)
	if err != nil {
		return err
	}
	ctx, cancel := interruptible()
	defer cancel()
	p := newProgress()
	for {
		select {
		case <-ctx.Done():
			eng.Cancel(j.ID)
			ctx = context.Background() // a second Ctrl-C kills the process
			p.done()
			fmt.Println("Stopping…")
		case ev := <-events:
			if ev.Job == nil || ev.Job.ID != j.ID {
				continue
			}
			p.show(*ev.Job)
			if ev.Job.State.Finished() {
				return p.finish(*ev.Job)
			}
		}
	}
}

// progress draws a job's progress on one terminal line.
type progress struct {
	tty   bool
	last  string
	key   string
	shown bool
}

func newProgress() *progress { return &progress{tty: isTerminal(os.Stdout)} }

func (p *progress) show(j engine.Job) {
	var line string
	switch j.State {
	case engine.StateFetching:
		line = "Fetching " + j.Label()
		if j.Pages > 0 {
			line += fmt.Sprintf(" — page %d, %s (%d new)", j.Pages, plural(j.Fetched, "item"), j.New)
		}
	case engine.StateDownloading:
		line = fmt.Sprintf("Downloading %s %s %d/%d (%d%%)", j.Label(), bar(j.Percent, 24), j.Current, j.Total, j.Percent)
	case engine.StateQueued:
		line = "Waiting in the queue…"
	default:
		return
	}
	if line == p.last {
		return
	}
	if p.tty {
		fmt.Printf("\r\033[K%s", line)
		p.shown = true
	} else {
		// Without a terminal, print whole lines, and fewer of them.
		key := string(j.State)
		switch j.State {
		case engine.StateDownloading:
			key += fmt.Sprint(j.Percent / 10)
		case engine.StateFetching:
			key += fmt.Sprint(j.Pages / 5)
		}
		if key != p.key {
			fmt.Println(line)
			p.key = key
		}
	}
	p.last = line
}

func (p *progress) done() {
	if p.tty && p.shown {
		fmt.Print("\r\033[K")
		p.shown = false
	}
}

func (p *progress) finish(j engine.Job) error {
	p.done()
	switch j.State {
	case engine.StateDone:
		fmt.Printf("✓ %s: %s\n", j.Label(), j.Message)
		if j.OutputDir != "" && j.Mode != engine.ModeFetch {
			fmt.Printf("  saved under %s\n", j.OutputDir)
		}
		return nil
	case engine.StateCancelled:
		fmt.Printf("✗ %s: %s\n", j.Label(), j.Message)
		return nil
	default:
		return fmt.Errorf("%s: %s", j.Label(), j.Error)
	}
}

func bar(percent, width int) string {
	n := percent * width / 100
	return "[" + strings.Repeat("█", n) + strings.Repeat("·", width-n) + "]"
}

func cmdJobs(args []string) error {
	fs := flag.NewFlagSet("jobs", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	c, err := client.Connect()
	if err != nil {
		return err
	}
	jobs, err := c.Jobs(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}
	if len(jobs) == 0 {
		fmt.Println("No jobs.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTARGET\tMODE\tSTATE\tPROGRESS\tFROM\tCREATED")
	for _, j := range jobs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", j.ID, j.Label(), j.Mode, j.State,
			truncate(j.Summary(), 48), j.Source, j.CreatedAt.Format(time.DateTime))
	}
	return tw.Flush()
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	c, err := client.Connect()
	if err != nil {
		return err
	}
	id := ""
	if len(pos) > 0 {
		id = pos[0]
	}
	ctx, cancel := interruptible()
	defer cancel()
	fmt.Println("Watching XDL's jobs (Ctrl-C to stop)…")
	last := map[string]string{}
	err = c.Events(ctx, id, func(ev engine.Event) bool {
		if ev.Job == nil {
			return true
		}
		j := *ev.Job
		if ev.Removed {
			return true
		}
		line := j.Summary()
		if last[j.ID] == line {
			return true
		}
		// Print downloads in steps of 5%, everything else as it comes.
		if j.State == engine.StateDownloading && j.Percent%5 != 0 && j.Current != j.Total {
			return true
		}
		last[j.ID] = line
		fmt.Printf("%s  %s  %-28s %s\n", time.Now().Format(time.TimeOnly), j.ID, j.Label(), line)
		return !(id != "" && j.State.Finished())
	})
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func cmdCancel(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: xdl cancel <job-id>")
	}
	c, err := client.Connect()
	if err != nil {
		return err
	}
	ok, err := c.Cancel(context.Background(), args[0])
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("the job has already finished")
	}
	fmt.Println("Cancelled", args[0])
	return nil
}

func cmdClear(args []string) error {
	c, err := client.Connect()
	if err != nil {
		return err
	}
	n, err := c.Clear(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("Removed %s.\n", plural(n, "finished job"))
	return nil
}

func cmdStatus(args []string) error {
	c, err := client.Connect()
	if err != nil {
		return err // a non-zero exit, for health checks
	}
	st, err := c.Status(context.Background())
	if err != nil {
		return err
	}
	kind := "app"
	if st.Kind == "serve" {
		kind = "xdl serve"
	}
	fmt.Printf("XDL %s is running (%s, pid %d) on http://%s\n", st.Version, kind, st.PID, c.Info.Address)
	fmt.Printf("  jobs: %d running, %d queued\n", st.Running, st.Queued)
	if st.Active != nil {
		fmt.Printf("  now:  %s — %s\n", st.Active.Label(), st.Active.Summary())
	}
	fmt.Printf("  auth token: %s, xtractor: %s\n", have(st.AuthToken, "saved"), have(st.Extractor, "installed"))
	fmt.Printf("  downloads go to %s\n", st.DownloadPath)
	return nil
}

func have(b bool, yes string) string {
	if b {
		return yes
	}
	return "missing"
}
