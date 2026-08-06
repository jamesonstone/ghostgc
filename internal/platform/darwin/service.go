//go:build darwin

package darwin

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// launchdPlist is the LaunchAgent template.
//
// KeepAlive is deliberately a dictionary rather than `true`: an unconditional
// KeepAlive turns a crash-on-startup bug into a crash loop that launchd will
// happily sustain forever. Restarting only on unsuccessful exit, combined with
// a throttle interval, bounds the damage.
const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
		<key>Crashed</key>
		<true/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>30</integer>
	<key>ExitTimeOut</key>
	<integer>20</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>Nice</key>
	<integer>5</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`

func plistPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func renderLaunchdPlist(label, binaryPath string, arguments []string) string {
	var program strings.Builder
	for _, arg := range append([]string{binaryPath}, arguments...) {
		_, _ = fmt.Fprintf(&program, "\t\t<string>%s</string>\n", esc(arg))
	}
	return fmt.Sprintf(launchdPlist,
		esc(label),
		strings.TrimSuffix(program.String(), "\n"),
		"/dev/null",
		"/dev/null",
	)
}

// InstallService writes the LaunchAgent property list and bootstraps it.
func (c *Collector) InstallService(ctx context.Context, label, binaryPath string, arguments []string, logDir string) error {
	if label == "" || binaryPath == "" {
		return fmt.Errorf("darwin: service label and binary path are required")
	}
	abs, err := filepath.Abs(binaryPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("darwin: service executable %s: %w", abs, err)
	}
	path, err := plistPath(label)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return err
	}
	content := renderLaunchdPlist(label, abs, arguments)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	// Remove any previous registration so that bootstrap is idempotent.
	_ = run(ctx, "launchctl", "bootout", domain+"/"+label)
	if err := run(ctx, "launchctl", "bootstrap", domain, path); err != nil {
		return fmt.Errorf("darwin: launchctl bootstrap: %w", err)
	}
	return nil
}

// UninstallService removes the LaunchAgent registration and property list.
func (c *Collector) UninstallService(ctx context.Context, label string) error {
	path, err := plistPath(label)
	if err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	// A missing registration is not an error; the goal state is "not running".
	_ = run(ctx, "launchctl", "bootout", domain+"/"+label)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ServiceStatus reports the launchd registration state.
func (c *Collector) ServiceStatus(ctx context.Context, label string) (installed, running bool, unitPath string, pid, lastExit int, err error) {
	unitPath, err = plistPath(label)
	if err != nil {
		return false, false, "", 0, 0, err
	}
	if _, statErr := os.Stat(unitPath); statErr != nil {
		return false, false, unitPath, 0, 0, nil
	}
	installed = true

	out, listErr := exec.CommandContext(ctx, "launchctl", "list", label).Output()
	if listErr != nil {
		// Registered on disk but not loaded into the session.
		return installed, false, unitPath, 0, 0, nil
	}
	pid, lastExit = parseLaunchctlList(out)
	running = pid > 0
	return installed, running, unitPath, pid, lastExit, nil
}

func parseLaunchctlList(out []byte) (pid, lastExit int) {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSuffix(strings.TrimSpace(value), ";")
		key = strings.Trim(strings.TrimSpace(key), `"`)
		switch key {
		case "PID":
			if n, convErr := strconv.Atoi(value); convErr == nil {
				pid = n
			}
		case "LastExitStatus":
			if n, convErr := strconv.Atoi(value); convErr == nil {
				lastExit = n
			}
		}
	}
	return pid, lastExit
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
