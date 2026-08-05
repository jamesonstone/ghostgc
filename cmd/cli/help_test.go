package main

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
)

var helpANSIPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRootHelpIsGroupedAndCompact(t *testing.T) {
	output, code := captureHelpRun(t, nil, false)
	if code != 0 {
		t.Fatalf("no-argument exit code = %d, want 0", code)
	}
	for _, line := range rootGhostArt {
		if !strings.Contains(output, line) {
			t.Fatalf("root help is missing ghost art %q:\n%s", line, output)
		}
	}
	for _, want := range []string{
		"ghostgc ",
		"Session-aware process observation for coding agents",
		"Local-first • explainable • fail-closed",
		"Usage",
		"Available Commands",
		"Run",
		"start",
		"stop",
		"Observe",
		"Policy & Cleanup",
		"Worktrees",
		"Inspect",
		"Setup",
		"Utilities",
		"--config <path>",
		"ghostgc <command> -h",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("root help is missing %q:\n%s", want, output)
		}
	}
	for _, noise := range []string{"cleanup --dry-run", "actions [--process", "classifications [--session"} {
		if strings.Contains(output, noise) {
			t.Fatalf("root help contains detailed syntax %q:\n%s", noise, output)
		}
	}
	if helpANSIPattern.MatchString(output) {
		t.Fatalf("non-terminal help contains ANSI escapes: %q", output)
	}
}

func TestRootHelpSectionsCoverEveryCommandOnce(t *testing.T) {
	seen := make(map[string]bool, len(commands))
	for _, section := range rootHelpSections {
		for _, name := range section.commands {
			if seen[name] {
				t.Fatalf("command %q appears in more than one help section", name)
			}
			if _, ok := commands[name]; !ok {
				t.Fatalf("help section references unknown command %q", name)
			}
			seen[name] = true
		}
	}
	for name := range commands {
		if !seen[name] {
			t.Fatalf("command %q is missing from root help sections", name)
		}
	}
}

func TestRootHelpColorsOnlyInteractiveOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	terminal, code := captureHelpRun(t, []string{"--help"}, true)
	if code != 0 {
		t.Fatalf("--help exit code = %d, want 0", code)
	}
	if !helpANSIPattern.MatchString(terminal) {
		t.Fatalf("interactive help has no ANSI styling: %q", terminal)
	}
	for _, want := range []string{strings.TrimSpace(rootGhostArt[0]), "ghostgc", "🚀 Usage", "🧰 Available Commands", "🌐 Global Flags"} {
		if !strings.Contains(stripHelpANSI(terminal), want) {
			t.Fatalf("interactive help is missing %q: %q", want, terminal)
		}
	}

	t.Setenv("NO_COLOR", "1")
	plain, _ := captureHelpRun(t, []string{"--help"}, true)
	if helpANSIPattern.MatchString(plain) {
		t.Fatalf("NO_COLOR help contains ANSI escapes: %q", plain)
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	dumbTerminal, _ := captureHelpRun(t, []string{"--help"}, true)
	if helpANSIPattern.MatchString(dumbTerminal) {
		t.Fatalf("TERM=dumb help contains ANSI escapes: %q", dumbTerminal)
	}
}

func TestUnknownCommandRemainsUsageError(t *testing.T) {
	output, code := captureHelpRun(t, []string{"not-a-command"}, false)
	if code != usageExitCode {
		t.Fatalf("unknown command exit code = %d, want %d", code, usageExitCode)
	}
	if !strings.Contains(output, `ghostgc: unknown command "not-a-command"`) {
		t.Fatalf("unknown command output = %q", output)
	}
}

func TestSuccessfulHelpUsesHelpOutput(t *testing.T) {
	previousHelp := cliHelpOutput
	previousError := cliErrorOutput
	previousTerminal := terminalWriterCheck
	defer func() {
		cliHelpOutput = previousHelp
		cliErrorOutput = previousError
		terminalWriterCheck = previousTerminal
	}()

	var helpOutput, errorOutput bytes.Buffer
	cliHelpOutput = &helpOutput
	cliErrorOutput = &errorOutput
	terminalWriterCheck = func(io.Writer) bool { return false }
	if code := run(context.Background(), []string{"--help"}); code != 0 {
		t.Fatalf("--help exit code = %d, want 0", code)
	}
	if helpOutput.Len() == 0 || errorOutput.Len() != 0 {
		t.Fatalf("help bytes = %d, error bytes = %d", helpOutput.Len(), errorOutput.Len())
	}

	helpOutput.Reset()
	errorOutput.Reset()
	if code := run(context.Background(), []string{"not-a-command"}); code != usageExitCode {
		t.Fatalf("unknown command exit code = %d, want %d", code, usageExitCode)
	}
	if helpOutput.Len() != 0 || errorOutput.Len() == 0 {
		t.Fatalf("unknown help bytes = %d, error bytes = %d", helpOutput.Len(), errorOutput.Len())
	}
}

func TestCommandHelpRetainsDetailedUsage(t *testing.T) {
	previousHelp := cliHelpOutput
	previousOutput := cliErrorOutput
	previousTerminal := terminalWriterCheck
	defer func() {
		cliHelpOutput = previousHelp
		cliErrorOutput = previousOutput
		terminalWriterCheck = previousTerminal
	}()

	var output bytes.Buffer
	cliHelpOutput = &output
	cliErrorOutput = &output
	terminalWriterCheck = func(io.Writer) bool { return false }
	fs := newFlagSet(&env{}, "actions", "[--process <pid:start>] [--limit <n>]")
	fs.String("process", "", "exact process key")
	fs.Int("limit", 0, "maximum actions to return")
	fs.Usage()

	for _, want := range []string{"Usage", "ghostgc actions [--process <pid:start>] [--limit <n>]", "Flags", "--process <string>"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("command help is missing %q:\n%s", want, output.String())
		}
	}
}

func TestLogsHelpShowsLongAndShortFollowFlags(t *testing.T) {
	output, code := captureHelpRun(t, []string{"logs", "-h"}, false)
	if code != 0 {
		t.Fatalf("logs help exit code = %d, want 0", code)
	}
	for _, want := range []string{"--follow", "-f", "default true"} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs help is missing %q:\n%s", want, output)
		}
	}
}

func captureHelpRun(t *testing.T, args []string, terminal bool) (string, int) {
	t.Helper()
	previousHelp := cliHelpOutput
	previousOutput := cliErrorOutput
	previousTerminal := terminalWriterCheck
	defer func() {
		cliHelpOutput = previousHelp
		cliErrorOutput = previousOutput
		terminalWriterCheck = previousTerminal
	}()

	var output bytes.Buffer
	cliHelpOutput = &output
	cliErrorOutput = &output
	terminalWriterCheck = func(io.Writer) bool { return terminal }
	code := run(context.Background(), args)
	return output.String(), code
}

func stripHelpANSI(value string) string {
	return helpANSIPattern.ReplaceAllString(value, "")
}
