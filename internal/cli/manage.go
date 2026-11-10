package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/deps"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/logbuf"
	"github.com/missuo/xdl/internal/schedule"
	"github.com/missuo/xdl/internal/server"
	"github.com/missuo/xdl/internal/settings"
	"github.com/missuo/xdl/internal/version"
)

func cmdLibrary(args []string) error {
	sub := "ls"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	if err := initDB(); err != nil {
		return err
	}
	switch sub {
	case "ls", "list":
		fs := flag.NewFlagSet("library ls", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "print JSON")
		if _, err := parse(fs, args); err != nil {
			return err
		}
		accounts, err := backend.GetAllAccounts()
		if err != nil {
			return err
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(accounts)
		}
		if len(accounts) == 0 {
			fmt.Println("The library is empty.")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tACCOUNT\tNAME\tTIMELINE\tITEMS\tCOMPLETE\tFETCHED")
		for _, a := range accounts {
			fmt.Fprintf(tw, "%d\t@%s\t%s\t%s\t%d\t%v\t%s\n", a.ID, a.Username, truncate(a.Name, 24),
				a.MediaType, a.TotalMedia, a.Completed, a.LastFetched)
		}
		return tw.Flush()
	case "rm", "remove", "delete":
		if len(args) == 0 {
			return errors.New("usage: xdl library rm <id>…")
		}
		for _, arg := range args {
			id, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", arg)
			}
			if err := backend.DeleteAccount(id); err != nil {
				return err
			}
			fmt.Println("Removed", id)
		}
		return nil
	case "export":
		fs := flag.NewFlagSet("library export", flag.ContinueOnError)
		out := fs.String("out", ".", "directory to write the export to")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: xdl library export <id> [-out dir]")
		}
		id, err := strconv.ParseInt(pos[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q", pos[0])
		}
		path, err := backend.ExportAccountToFile(id, *out)
		if err != nil {
			return err
		}
		fmt.Println("Exported to", path)
		return nil
	case "import":
		if len(args) != 1 {
			return errors.New("usage: xdl library import <file.json>")
		}
		user, err := backend.ImportAccountFromFile(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Imported @%s\n", user)
		return nil
	}
	return fmt.Errorf("unknown library command %q (ls, rm, export, import)", sub)
}

func cmdToken(args []string) error {
	sub := "show"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "show":
		token, err := backend.GetStoredAuthToken("public")
		if err != nil {
			return err
		}
		if token == "" {
			fmt.Println("No auth token saved. Run `xdl token set`.")
			return nil
		}
		fmt.Printf("auth_token: %s…%s (%d characters)\n", token[:min(4, len(token))], token[max(0, len(token)-4):], len(token))
		return nil
	case "set":
		var token string
		if len(args) > 0 {
			token = args[0]
		} else {
			var err error
			if token, err = readSecret("X auth_token: "); err != nil {
				return err
			}
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return errors.New("no token given")
		}
		if len(token) != 40 {
			fmt.Fprintf(os.Stderr, "warning: auth_token cookies are usually 40 characters; this one has %d\n", len(token))
		}
		if err := backend.SetStoredAuthToken("public", token); err != nil {
			return err
		}
		fmt.Println("Saved the auth token to the Keychain.")
		return nil
	case "clear":
		if err := backend.ClearStoredAuthToken("public"); err != nil {
			return err
		}
		fmt.Println("Removed the auth token.")
		return nil
	}
	return fmt.Errorf("unknown token command %q (show, set, clear)", sub)
}

// readSecret reads a line from the terminal without echoing it.
func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	tty := isTerminal(os.Stdin) && runtime.GOOS != "windows"
	if tty {
		stty := func(arg string) {
			cmd := exec.Command("stty", arg)
			cmd.Stdin = os.Stdin
			_ = cmd.Run()
		}
		stty("-echo")
		defer func() {
			stty("echo")
			fmt.Fprintln(os.Stderr)
		}()
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func cmdConfig(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	store := settings.Open()
	switch sub {
	case "list", "ls":
		data, _ := json.MarshalIndent(store.Get(), "", "  ")
		fmt.Println(string(data))
		return nil
	case "path":
		fmt.Println(settings.Path())
		return nil
	case "get":
		if len(args) != 1 {
			return errors.New("usage: xdl config get <key>")
		}
		data, _ := json.Marshal(store.Get())
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		v, ok := m[args[0]]
		if !ok {
			return fmt.Errorf("unknown setting %q (known: %s)", args[0], strings.Join(settings.Keys(), ", "))
		}
		fmt.Println(v)
		return nil
	case "set":
		if len(args) != 2 {
			return errors.New("usage: xdl config set <key> <value>")
		}
		var setErr error
		next, err := store.Update(func(s *settings.Settings) { setErr = settings.SetKey(s, args[0], args[1]) })
		if setErr != nil {
			return setErr
		}
		if err != nil {
			return err
		}
		data, _ := json.Marshal(next)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		fmt.Printf("%s = %v\n", args[0], m[args[0]])
		return nil
	case "reset":
		if err := store.Set(settings.Defaults()); err != nil {
			return err
		}
		fmt.Println("Restored the default settings.")
		return nil
	}
	return fmt.Errorf("unknown config command %q (list, get, set, path, reset)", sub)
}

func cmdDeps(args []string) error {
	sub := "status"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "status", "ls":
		type row struct {
			dep deps.Dep
			st  deps.Status
		}
		rows := make([]row, len(deps.All))
		var wg sync.WaitGroup
		for i, d := range deps.All {
			wg.Go(func() { rows[i] = row{d, d.Status()} })
		}
		wg.Wait()
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "TOOL\tINSTALLED\tLATEST\tNOTE")
		for _, r := range rows {
			installed := "no"
			if r.st.Installed {
				installed = cmpOr(r.st.InstalledVersion, "yes")
			}
			note := r.dep.Description
			if r.st.System {
				note += " (system install)"
			} else if r.st.Installed && !r.st.UpToDate() {
				note += " — update available"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.dep.Title, installed, cmpOr(r.st.LatestVersion, "?"), note)
		}
		return tw.Flush()
	case "install", "update":
		if len(args) == 0 {
			return errors.New("usage: xdl deps install <ffmpeg|exiftool|all>")
		}
		var list []deps.Dep
		if args[0] == "all" {
			list = deps.All
		} else {
			for _, name := range args {
				d, err := deps.Find(name)
				if err != nil {
					return err
				}
				list = append(list, d)
			}
		}
		for _, d := range list {
			if err := installDep(d); err != nil {
				return fmt.Errorf("%s: %w", d.Title, err)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown deps command %q (status, install)", sub)
}

func installDep(d deps.Dep) error {
	tty := isTerminal(os.Stdout)
	fmt.Printf("Installing %s…\n", d.Title)
	var last time.Time
	err := d.Download(func(done, total int64) {
		if !tty || (time.Since(last) < 100*time.Millisecond && done != total) {
			return
		}
		last = time.Now()
		if total > 0 {
			pct := int(done * 100 / total)
			fmt.Printf("\r\033[K  %s %d%% (%.1f / %.1f MB)", bar(pct, 24), pct, float64(done)/1e6, float64(total)/1e6)
		} else {
			fmt.Printf("\r\033[K  %.1f MB", float64(done)/1e6)
		}
	})
	if tty {
		fmt.Print("\r\033[K")
	}
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s installed\n", d.Title)
	return nil
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	_ = fs.Bool("install-deps", false, "accepted for older setups; XDL needs nothing installed")
	addr := fs.String("addr", envOr("XDL_API_ADDR", server.DefaultAddr), "listen address; another than 127.0.0.1 requires the API token on every request (the extension expects "+server.DefaultAddr+")")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	logbuf.Default.SetOutput(os.Stderr)
	if err := initDB(); err != nil {
		return err
	}
	ctx, cancel := interruptible()
	defer cancel()
	eng := engine.New(settings.Open())
	go eng.Run(ctx)
	sched := schedule.New(eng)
	srv := server.New(eng, sched, *addr, version.Version, "serve")
	if err := srv.Listen(); err != nil {
		return err
	}
	go sched.Run(ctx)
	fmt.Fprintf(os.Stderr, "XDL %s serving on http://%s (Ctrl-C to stop)\n", version.Version, srv.Addr())
	if os.Getenv("XDL_API_TOKEN") == "" && !isLoopbackAddr(srv.Addr()) {
		fmt.Fprintf(os.Stderr, "API token (set XDL_API_TOKEN to choose one): %s\n", srv.Token())
	}
	if token, _ := backend.GetStoredAuthToken("public"); token == "" {
		fmt.Fprintln(os.Stderr, "warning: no X auth token — set XDL_AUTH_TOKEN or run `xdl token set`")
	}
	err := srv.Serve(ctx)
	backend.CloseDB()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func cmdHistory(args []string) error {
	sub := "count"
	if len(args) > 0 {
		sub = args[0]
	}
	if err := initDB(); err != nil {
		return err
	}
	switch sub {
	case "count", "show":
		n, err := backend.DownloadHistoryCount()
		if err != nil {
			return err
		}
		on := settings.Open().Get().DownloadHistory
		fmt.Printf("The download history holds %s; skipping them is %s.\n", plural(n, "item"), map[bool]string{true: "on", false: "off (xdl config set download_history true)"}[on])
		return nil
	case "clear":
		n, err := backend.ClearDownloadHistory()
		if err != nil {
			return err
		}
		fmt.Printf("Forgot %s. Files already on disk are still skipped.\n", plural(int(n), "download"))
		return nil
	}
	return fmt.Errorf("unknown history command %q (count, clear)", sub)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
