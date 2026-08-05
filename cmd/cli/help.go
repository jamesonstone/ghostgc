package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/version"
	"github.com/mattn/go-isatty"
)

const (
	helpReset     = "\x1b[0m"
	helpBoldWhite = "\x1b[1;37m"
	helpCyan      = "\x1b[38;5;39m"
	helpDim       = "\x1b[38;5;245m"
)

var (
	cliHelpOutput       io.Writer = os.Stdout
	cliErrorOutput      io.Writer = os.Stderr
	terminalWriterCheck           = isTerminalWriter
)

var rootGhostArt = [...]string{
	`        .-""""""-.`,
	`      .'          '.`,
	`     /   (O)  (o)   \`,
	`    |       ^        |`,
	` .--|    \_____/     |--.`,
	` /  |       U        |  \`,
	` \  |                |  /`,
	"  `-'\\              /'-'",
	`      \   /\  /\   /`,
	"       `-'  \\/  `-'",
}

type helpSection struct {
	title    string
	commands []string
}

var rootHelpSections = []helpSection{
	{title: "Run", commands: []string{"start", "stop"}},
	{title: "Observe", commands: []string{"status", "sessions", "session", "processes", "explain", "activity", "classifications"}},
	{title: "Policy & Cleanup", commands: []string{"candidates", "policies", "policy", "cleanup", "actions"}},
	{title: "Cache Lifecycle", commands: []string{"cache"}},
	{title: "Worktrees", commands: []string{"worktrees", "worktree"}},
	{title: "Inspect", commands: []string{"logs", "metrics", "doctor"}},
	{title: "Setup", commands: []string{"config", "service", "daemon"}},
	{title: "Utilities", commands: []string{"version"}},
}

type helpStyle struct {
	enabled bool
}

func styleForHelp(w io.Writer) helpStyle {
	enabled := terminalWriterCheck(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return helpStyle{enabled: enabled}
}

func isTerminalWriter(w io.Writer) bool {
	fdWriter, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	fd := fdWriter.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func (s helpStyle) title(icon, text string) string {
	if !s.enabled {
		return text
	}
	return helpBoldWhite + icon + " " + text + helpReset
}

func (s helpStyle) label(text string) string {
	if !s.enabled {
		return text
	}
	return helpBoldWhite + text + helpReset
}

func (s helpStyle) accent(text string) string {
	if !s.enabled {
		return text
	}
	return helpCyan + text + helpReset
}

func (s helpStyle) muted(text string) string {
	if !s.enabled {
		return text
	}
	return helpDim + text + helpReset
}

func printUsage(global *flag.FlagSet, out io.Writer) {
	style := styleForHelp(out)

	printRootBanner(out, style)
	_, _ = fmt.Fprintf(out, "\n%s\n  ghostgc [global flags] <command> [flags]\n", style.title("🚀", "Usage"))
	_, _ = fmt.Fprintf(out, "\n%s\n", style.title("🧰", "Available Commands"))

	padding := rootHelpNamePadding()
	for _, section := range rootHelpSections {
		_, _ = fmt.Fprintf(out, "\n%s\n", style.label(section.title))
		for _, name := range section.commands {
			command, ok := commands[name]
			if !ok {
				continue
			}
			_, _ = fmt.Fprintf(out, "  %-*s %s\n", padding, command.name, command.summary)
		}
	}

	_, _ = fmt.Fprintf(out, "\n%s\n", style.title("🌐", "Global Flags"))
	printFlagRows(out, global, rootFlagPlaceholder)
	_, _ = fmt.Fprintf(out, "\n%s \"ghostgc <command> -h\" for detailed command help.\n", helpMoreInfoLabel(style))
}

func printRootBanner(out io.Writer, style helpStyle) {
	for index, art := range rootGhostArt {
		var text string
		switch index {
		case 1:
			text = style.label("ghostgc") + " " + style.accent(version.String())
		case 2:
			text = style.muted("Session-aware process observation for coding agents")
		case 3:
			text = style.muted("Local-first • explainable • fail-closed")
		}
		if text == "" {
			_, _ = fmt.Fprintln(out, style.accent(art))
			continue
		}
		_, _ = fmt.Fprintf(out, "%s%s\n", style.accent(fmt.Sprintf("%-28s", art)), text)
	}
}

func rootHelpNamePadding() int {
	width := 0
	for _, section := range rootHelpSections {
		for _, name := range section.commands {
			if len(name) > width {
				width = len(name)
			}
		}
	}
	return width + 2
}

type flagPlaceholder func(*flag.Flag) string

func printFlagRows(out io.Writer, fs *flag.FlagSet, placeholder flagPlaceholder) {
	type row struct {
		name        string
		description string
	}
	var rows []row
	width := 0
	fs.VisitAll(func(item *flag.Flag) {
		prefix := "--"
		if len(item.Name) == 1 {
			prefix = "-"
		}
		name := prefix + item.Name
		if value := placeholder(item); value != "" {
			name += " " + value
		}
		_, usage := flag.UnquoteUsage(item)
		if defaultValue := visibleFlagDefault(item); defaultValue != "" {
			usage += " (default " + defaultValue + ")"
		}
		rows = append(rows, row{name: name, description: usage})
		if len(name) > width {
			width = len(name)
		}
	})
	for _, item := range rows {
		_, _ = fmt.Fprintf(out, "  %-*s  %s\n", width, item.name, item.description)
	}
}

func rootFlagPlaceholder(item *flag.Flag) string {
	if boolean, ok := item.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
		return ""
	}
	if item.Name == "config" || item.Name == "socket" {
		return "<path>"
	}
	return "<value>"
}

func commandFlagPlaceholder(item *flag.Flag) string {
	name, _ := flag.UnquoteUsage(item)
	if name == "" {
		return ""
	}
	return "<" + name + ">"
}

func visibleFlagDefault(item *flag.Flag) string {
	switch item.DefValue {
	case "", "0", "false":
		return ""
	default:
		return item.DefValue
	}
}

func helpMoreInfoLabel(style helpStyle) string {
	if style.enabled {
		return style.label("🔎 Use")
	}
	return "Use"
}

func newFlagSet(_ *env, name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet("ghostgc "+name, flag.ContinueOnError)
	fs.SetOutput(cliErrorOutput)
	fs.Usage = func() {
		out := cliHelpOutput
		style := styleForHelp(out)
		useLine := strings.TrimSpace("ghostgc " + name + " " + usage)
		_, _ = fmt.Fprintf(out, "%s\n  %s\n", style.title("🚀", "Usage"), useLine)
		if fs.NFlag() > 0 || hasDefinedFlags(fs) {
			_, _ = fmt.Fprintf(out, "\n%s\n", style.title("⚙️", "Flags"))
			printFlagRows(out, fs, commandFlagPlaceholder)
		}
	}
	return fs
}

func hasDefinedFlags(fs *flag.FlagSet) bool {
	found := false
	fs.VisitAll(func(_ *flag.Flag) { found = true })
	return found
}
