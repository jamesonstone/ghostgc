package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppName is the short name used for directories, the socket and the CLI.
const AppName = "ghostgc"

// ServiceLabel is the launchd / systemd unit label.
const ServiceLabel = "com.github.jamesonstone.ghostgc"

// maxUnixSocketPath is the practical limit for sun_path on macOS. Exceeding it
// produces an opaque "invalid argument" at bind time, so it is checked up front.
const maxUnixSocketPath = 100

// Paths resolves every location the daemon and CLI use.
type Paths struct {
	Config   string
	StateDir string
	LogDir   string
	Socket   string
	Database string
}

// DefaultPaths returns the platform-appropriate locations.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("config: resolving home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", AppName)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && runtime.GOOS != "darwin" {
		configDir = filepath.Join(xdg, AppName)
	}

	var stateDir, logDir, socket string
	switch runtime.GOOS {
	case "darwin":
		stateDir = filepath.Join(home, "Library", "Application Support", AppName)
		logDir = filepath.Join(home, "Library", "Logs", AppName)
		socket = filepath.Join(stateDir, AppName+".sock")
	default:
		stateDir = filepath.Join(home, ".local", "state", AppName)
		if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
			stateDir = filepath.Join(xdg, AppName)
		}
		logDir = filepath.Join(stateDir, "logs")
		socket = filepath.Join(stateDir, AppName+".sock")
		if runDir := os.Getenv("XDG_RUNTIME_DIR"); runDir != "" {
			socket = filepath.Join(runDir, AppName+".sock")
		}
	}

	return Paths{
		Config:   filepath.Join(configDir, "config.yaml"),
		StateDir: stateDir,
		LogDir:   logDir,
		Socket:   socket,
		Database: filepath.Join(stateDir, AppName+".db"),
	}, nil
}

// Validate checks the resolved paths for problems that would otherwise surface
// as opaque failures much later.
func (p Paths) Validate() error {
	if len(p.Socket) > maxUnixSocketPath {
		return fmt.Errorf("config: socket path is %d bytes, which exceeds the %d byte limit for unix sockets: %s",
			len(p.Socket), maxUnixSocketPath, p.Socket)
	}
	return nil
}

// EnsureDirs creates the state and log directories with private permissions.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.StateDir, p.LogDir, filepath.Dir(p.Config)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("config: creating %s: %w", dir, err)
		}
	}
	return nil
}
