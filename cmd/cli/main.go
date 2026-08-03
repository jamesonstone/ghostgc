// Command ghostgc inspects and controls the ghostgc daemon.
//
// Every command in this build reads. The commands that would act — cleanup,
// policy enable — exist so that their absence is explicit rather than
// mysterious, and they refuse with a message naming the delivery phase that
// will introduce them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/version"
)

const usageExitCode = 2

type env struct {
	paths      config.Paths
	configPath string
	socket     string
	jsonOut    bool

	client *api.Client
}

func (e *env) api() *api.Client {
	if e.client == nil {
		e.client = api.NewClient(e.socket)
	}
	return e.client
}

type command struct {
	name    string
	summary string
	usage   string
	run     func(ctx context.Context, e *env, args []string) error
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code := run(ctx, os.Args[1:])
	os.Exit(code)
}

func run(ctx context.Context, argv []string) int {
	e := &env{}
	global := flag.NewFlagSet("ghostgc", flag.ContinueOnError)
	global.SetOutput(os.Stderr)
	global.StringVar(&e.configPath, "config", "", "path to config.yaml")
	global.StringVar(&e.socket, "socket", "", "path to the daemon socket")
	global.BoolVar(&e.jsonOut, "json", false, "emit JSON instead of human-readable output")
	global.Usage = func() { printUsage(global) }

	if err := global.Parse(argv); err != nil {
		return usageExitCode
	}
	args := global.Args()
	if len(args) == 0 {
		printUsage(global)
		return usageExitCode
	}

	if err := e.resolvePaths(); err != nil {
		fmt.Fprintln(os.Stderr, "ghostgc:", err)
		return 1
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "ghostgc: unknown command %q\n\n", name)
		printUsage(global)
		return usageExitCode
	}
	if err := cmd.run(ctx, e, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return usageExitCode
		}
		fmt.Fprintln(os.Stderr, "ghostgc:", err)
		if errors.Is(err, api.ErrDaemonUnreachable) {
			fmt.Fprintln(os.Stderr, "\nStart it with `ghostgc service install`, or run `ghostgcd` in the foreground.")
		}
		return 1
	}
	return 0
}

func (e *env) resolvePaths() error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	if e.configPath != "" {
		paths.Config = e.configPath
	}
	// A configuration that fails validation must not stop read-only commands
	// such as `doctor`, which is the command a user runs to find out why.
	if cfg, err := config.Load(paths.Config); err == nil {
		paths = cfg.ResolvePaths(paths)
	}
	if e.socket != "" {
		paths.Socket = e.socket
	}
	e.paths = paths
	e.socket = paths.Socket
	return nil
}

var commands map[string]command

func init() {
	list := []command{
		{
			name:    "status",
			summary: "show daemon health, mode and the last scan",
			run:     cmdStatus,
		},
		{
			name:    "sessions",
			summary: "list agent sessions",
			usage:   "[--state <state>] [--agent <id>] [--limit <n>]",
			run:     cmdSessions,
		},
		{
			name:    "session",
			summary: "show one session in detail",
			usage:   "show <session-id>",
			run:     cmdSession,
		},
		{
			name:    "processes",
			summary: "list processes attributed to agent sessions",
			usage:   "[--session <id>] [--all] [--limit <n>]",
			run:     cmdProcesses,
		},
		{
			name:    "explain",
			summary: "explain what ghostgc concluded about a pid, and why",
			usage:   "<pid>",
			run:     cmdExplain,
		},
		{
			name:    "candidates",
			summary: "list cleanup candidates",
			run:     cmdCandidates,
		},
		{
			name:    "policies",
			summary: "list cleanup policies",
			run:     cmdPolicies,
		},
		{
			name:    "policy",
			summary: "enable or disable a cleanup policy",
			usage:   "enable|disable <policy-id>",
			run:     cmdPolicy,
		},
		{
			name:    "cleanup",
			summary: "evaluate and apply cleanup policies",
			usage:   "--dry-run | --apply",
			run:     cmdCleanup,
		},
		{
			name:    "logs",
			summary: "show the audit log",
			usage:   "[--limit <n>] [--kind <kind>] [--subject <subject>]",
			run:     cmdLogs,
		},
		{
			name:    "doctor",
			summary: "diagnose the installation",
			run:     cmdDoctor,
		},
		{
			name:    "metrics",
			summary: "show daemon metrics",
			run:     cmdMetrics,
		},
		{
			name:    "activity",
			summary: "show targeted process activity evidence",
			usage:   "[--session <id>] [--process <proc-uid>] [--limit <n>]",
			run:     cmdActivity,
		},
		{
			name:    "classifications",
			summary: "show deterministic process classifications",
			usage:   "[--session <id>] [--process <proc-uid>] [--state <state>] [--history] [--limit <n>]",
			run:     cmdClassifications,
		},
		{
			name:    "config",
			summary: "manage the configuration file",
			usage:   "init|path|show",
			run:     cmdConfig,
		},
		{
			name:    "service",
			summary: "manage the background service registration",
			usage:   "install|uninstall|status",
			run:     cmdService,
		},
		{
			name:    "version",
			summary: "print the version",
			run:     cmdVersion,
		},
	}
	commands = make(map[string]command, len(list))
	for _, c := range list {
		commands[c.name] = c
	}
}

func printUsage(global *flag.FlagSet) {
	out := os.Stderr
	_, _ = fmt.Fprintf(out, "ghostgc %s — session-aware process observation for coding agents\n\n", version.String())
	_, _ = fmt.Fprintf(out, "Delivery phase %s\n\n", version.Phase)
	_, _ = fmt.Fprintln(out, "Usage:")
	_, _ = fmt.Fprintln(out, "  ghostgc [global flags] <command> [flags]")
	_, _ = fmt.Fprintln(out, "\nCommands:")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := commands[name]
		line := "  " + name
		if c.usage != "" {
			line += " " + c.usage
		}
		_, _ = fmt.Fprintf(out, "%-46s %s\n", line, c.summary)
	}
	_, _ = fmt.Fprintln(out, "\nGlobal flags:")
	global.PrintDefaults()
}

func newFlagSet(e *env, name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet("ghostgc "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ghostgc %s %s\n\n", name, usage)
		fs.PrintDefaults()
	}
	return fs
}
