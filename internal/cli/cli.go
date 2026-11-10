// Package cli is XDL's command line interface. The `xdl` binary runs it,
// and so does the app's own executable when it is given a command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/logbuf"
	"github.com/missuo/xdl/internal/version"
)

type command struct {
	name    string
	args    string
	summary string
	run     func(args []string) error
}

var commands []command

func init() {
	commands = []command{
		{"download", "<user|url> [flags]", "Fetch an account and download its media", cmdDownload},
		{"fetch", "<user|url> [flags]", "Fetch an account into the library without downloading", cmdFetch},
		{"send", "<user|url> [flags]", "Queue a download in the running app and return", cmdSend},
		{"jobs", "[-json]", "List the running app's jobs", cmdJobs},
		{"watch", "[job-id]", "Follow the running app's jobs as they progress", cmdWatch},
		{"cancel", "<job-id>", "Cancel a queued or running job", cmdCancel},
		{"clear", "", "Forget finished jobs", cmdClear},
		{"sub", "[ls|add|rm|pause|resume|run] …", "Subscribe to accounts: check them on a schedule and download what is new", cmdSub},
		{"library", "[ls|rm|export] …", "List, remove or export saved accounts", cmdLibrary},
		{"history", "[count|clear]", "Show or clear the download history that skips media downloaded before", cmdHistory},
		{"token", "[set|show|clear]", "Manage the X auth_token (kept in the Keychain)", cmdToken},
		{"config", "[list|get|set|path]", "Show or change settings", cmdConfig},
		{"deps", "[status|install <name|all>]", "Check or install xtractor, FFmpeg and ExifTool", cmdDeps},
		{"mcp", "", "Serve MCP over stdio, for AI agents (`claude mcp add xdl -- xdl mcp`)", cmdMCP},
		{"serve", "[-addr host:port]", "Run the API and job queue without the GUI", cmdServe},
		{"status", "", "Show whether XDL is running and what it is doing", cmdStatus},
		{"version", "", "Print the version", func([]string) error { fmt.Println("xdl", version.Version); return nil }},
	}
}

// IsCommand reports whether arg names a CLI command, so the app's
// executable can tell a CLI invocation from a launch.
func IsCommand(arg string) bool {
	switch arg {
	case "help", "-h", "-help", "--help", "-v", "--version":
		return true
	}
	for _, c := range commands {
		if c.name == arg {
			return true
		}
	}
	return false
}

// Main runs the CLI with os.Args[1:] and returns the exit code.
func Main(args []string) int {
	// Log lines stay quiet unless asked for (serve turns them on).
	logbuf.Install("")
	if os.Getenv("XDL_DEBUG") == "" {
		logbuf.Default.SetOutput(nil)
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		if len(args) > 1 {
			for _, c := range commands {
				if c.name == args[1] {
					fmt.Printf("usage: xdl %s %s\n\n%s\n", c.name, c.args, c.summary)
					return 0
				}
			}
		}
		usage(os.Stdout)
		return 0
	}
	if args[0] == "-v" || args[0] == "--version" {
		fmt.Println("xdl", version.Version)
		return 0
	}
	for _, c := range commands {
		if c.name == args[0] {
			if err := c.run(args[1:]); err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return 0
				}
				fmt.Fprintln(os.Stderr, "xdl:", err)
				return 1
			}
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "xdl: unknown command %q\n\n", args[0])
	usage(os.Stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "XDL %s — batch downloader for X/Twitter media\n\nUsage: xdl <command> [arguments]\n\nCommands:\n", version.Version)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s %s\t%s\n", c.name, c.args, c.summary)
	}
	tw.Flush()
	fmt.Fprint(w, `
download and fetch hand the job to the running app (or `+"`xdl serve`"+`) when
there is one, so it shares the app's queue; -local runs it in this process.

Run 'xdl help <command>' or 'xdl <command> -h' for a command's flags.
`)
}

// parse parses flags that may come before or after the positional
// arguments, as `xdl download elonmusk -type likes`.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		if rest[0] == "--" {
			return append(positional, rest[1:]...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// interruptible returns a context cancelled by the first Ctrl-C.
func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func initDB() error {
	if err := backend.InitDB(); err != nil {
		return fmt.Errorf("opening the library (%s): %w", backend.GetDBPath(), err)
	}
	return nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
