// Command ghostgc runs, inspects and controls the ghostgc daemon.
//
// Most commands read. Cleanup is the sole action command and requires a
// short-lived approval emitted by an exact preview; runtime policy mutation
// remains unavailable.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
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
	global.SetOutput(cliErrorOutput)
	global.StringVar(&e.configPath, "config", "", "path to config.yaml")
	global.StringVar(&e.socket, "socket", "", "path to the daemon socket")
	global.BoolVar(&e.jsonOut, "json", false, "emit JSON instead of human-readable output")
	global.Usage = func() { printUsage(global, cliHelpOutput) }

	if err := global.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return usageExitCode
	}
	args := global.Args()
	if len(args) == 0 {
		printUsage(global, cliHelpOutput)
		return 0
	}

	if err := e.resolvePaths(); err != nil {
		_, _ = fmt.Fprintln(cliErrorOutput, "ghostgc:", err)
		return 1
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		_, _ = fmt.Fprintf(cliErrorOutput, "ghostgc: unknown command %q\n\n", name)
		printUsage(global, cliErrorOutput)
		return usageExitCode
	}
	if err := cmd.run(ctx, e, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(cliErrorOutput, "ghostgc:", err)
		if errors.Is(err, api.ErrDaemonUnreachable) {
			_, _ = fmt.Fprintln(cliErrorOutput, "\nStart it with `ghostgc service install`, or run `ghostgc daemon` in the foreground.")
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
			summary: "preview or apply one manually approved cleanup",
			usage:   "--dry-run --process <pid:start> --policy <id> | --apply --approval <token> --yes",
			run:     cmdCleanup,
		},
		{
			name:    "actions",
			summary: "show durable cleanup action history",
			usage:   "[--process <pid:start>] [--policy <id>] [--result <result>] [--limit <n>]",
			run:     cmdActions,
		},
		{
			name:    "cache",
			summary: "inspect and manage exact session-owned cache artifacts",
			usage:   "artifacts|explain|candidates|cleanup|quarantined|restore|purge|actions",
			run:     cmdCache,
		},
		{
			name:    "logs",
			summary: "show the audit log",
			usage:   "[--limit <n>] [--kind <kind>] [--subject <subject>]",
			run:     cmdLogs,
		},
		{
			name:    "daemon",
			summary: "run the observation daemon in the foreground",
			usage:   "[--config <path>] [--log-level <level>] [--once] [--version]",
			run:     cmdDaemon,
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
