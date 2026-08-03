// Package protection evaluates the hard protections from the specification.
//
// A hard protection is a reason a process must never be terminated
// automatically, and it cannot be overridden by an ordinary policy file. In
// this delivery phase nothing can terminate anything at all, so these rules do
// not gate an action — they are evaluated and reported so that `ghostgc
// explain` can tell the user, today, why a process would be refused once
// cleanup policies exist.
//
// Every rule here fails closed: when a fact needed to clear a protection is
// unavailable, the protection applies.
package protection

import (
	"fmt"
	"path"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// Rule is one triggered protection.
type Rule struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Result is the outcome of evaluating every protection against a process.
type Result struct {
	Protected bool   `json:"protected"`
	Rules     []Rule `json:"rules,omitempty"`
}

// Input is everything the evaluator needs. It is passed explicitly rather than
// read from globals so the rules stay unit-testable.
type Input struct {
	Process process.Process
	// SelfPID is the daemon's own PID.
	SelfPID int
	// SelfUID is the uid the daemon runs as.
	SelfUID uint32
	// IsAgentRoot reports whether this process is a live agent session root.
	IsAgentRoot bool
	// SessionActive reports whether the owning session is still running.
	SessionActive bool
	// AttributionConfidence is the confidence of the process's attribution.
	AttributionConfidence float64
	// DescendantCount is the number of live descendants.
	DescendantCount int
	// AdapterRules are the protections contributed by agent adapters.
	AdapterRules []adapters.ProtectionRule
}

// Categories of executable that are protected as a class. These are matched on
// executable basename, never on a substring of the command line.
//
// Section 14 of the specification is explicit that broad runtime names such as
// node, python, go and bash must not be enforceable cleanup targets. They are
// listed here so the refusal is visible and attributable to a rule rather than
// being an accident of policy authoring.
var execClasses = []struct {
	id     string
	reason string
	names  []string
}{
	{
		id:     "protected-editor-v1",
		reason: "interactive editors and IDEs hold unsaved user state",
		names: []string{
			"code", "code - insiders", "codium", "cursor", "zed", "windsurf",
			"idea", "goland", "pycharm", "webstorm", "rider", "clion",
			"phpstorm", "rubymine", "datagrip", "studio", "xcode",
			"nvim", "vim", "vi", "emacs", "helix", "hx", "sublime_text", "nova",
		},
	},
	{
		id:     "protected-language-server-v1",
		reason: "language servers are shared editor infrastructure, not agent work",
		names: []string{
			"gopls", "rust-analyzer", "clangd", "pyright", "pyright-langserver",
			"basedpyright-langserver", "typescript-language-server", "tsserver",
			"vtsls", "jdtls", "solargraph", "ruby-lsp", "lua-language-server",
			"yaml-language-server", "json-languageserver", "bash-language-server",
			"sourcekit-lsp", "metals", "elixir-ls", "terraform-ls", "tflint",
			"omnisharp", "intelephense", "svelteserver", "vscode-eslint-server",
		},
	},
	{
		id:     "protected-container-runtime-v1",
		reason: "container runtimes own state for every container on the machine",
		names: []string{
			"dockerd", "docker", "com.docker.backend", "com.docker.build",
			"containerd", "containerd-shim", "podman", "colima", "limactl",
			"lima", "vfkit", "krunkit", "qemu-system-aarch64", "qemu-system-x86_64",
			"rancher-desktop", "orbstack", "buildkitd",
		},
	},
	{
		id:     "protected-database-v1",
		reason: "database servers can lose or corrupt data when signalled",
		names: []string{
			"postgres", "postgresql", "mysqld", "mariadbd", "mongod",
			"redis-server", "valkey-server", "clickhouse-server", "cockroach",
			"etcd", "influxd", "cassandra", "elasticsearch", "opensearch",
			"memcached", "nats-server", "rabbitmq-server",
		},
	},
	{
		id:     "protected-build-or-test-v1",
		reason: "build and test processes represent in-flight work whose loss is expensive",
		names: []string{
			"make", "gmake", "cmake", "ninja", "bazel", "bazelisk", "buck2",
			"gradle", "gradlew", "mvn", "maven", "cargo", "rustc", "tsc",
			"webpack", "rollup", "esbuild", "swc", "turbo", "nx",
			"jest", "vitest", "pytest", "tox", "rspec", "phpunit", "gotestsum",
			"ctest", "xcodebuild", "swift", "dotnet",
		},
	},
	{
		id:     "protected-development-server-v1",
		reason: "development servers are long-lived by design; being idle is normal for them",
		names: []string{
			"next-server", "vite", "webpack-dev-server", "nodemon", "rails",
			"puma", "unicorn", "gunicorn", "uvicorn", "hypercorn", "daphne",
			"flask", "django-admin", "air", "watchexec", "entr",
		},
	},
	{
		id:     "protected-broad-runtime-v1",
		reason: "executable name is too broad to establish what the process is doing (specification section 14)",
		names: []string{
			"node", "bun", "deno", "python", "python3", "ruby", "perl", "php",
			"go", "java", "dotnet", "git", "bash", "zsh", "sh", "dash", "fish",
			"ksh", "tcsh", "csh", "nu", "xonsh", "ssh", "rsync", "tmux", "screen",
		},
	},
}

// Evaluate runs every hard protection against a process.
func Evaluate(in Input) Result {
	var rules []Rule
	add := func(id, format string, args ...any) {
		rules = append(rules, Rule{ID: id, Reason: fmt.Sprintf(format, args...)})
	}

	p := in.Process

	if in.SelfPID != 0 && p.PID == in.SelfPID {
		add("protected-self-v1", "process %d is the ghostgc daemon itself", p.PID)
	}
	if p.PID == process.InitPID {
		add("protected-init-v1", "process %d is the init process", p.PID)
	}
	if p.UID != in.SelfUID {
		add("protected-other-user-v1", "process is owned by uid %d, not by the user running ghostgc (uid %d)", p.UID, in.SelfUID)
	}
	if p.HasTTY() {
		add("protected-controlling-terminal-v1", "process has a controlling terminal (%s), which indicates an interactive session", p.TTY)
	}
	if p.PID == p.SID && p.SID != 0 {
		add("protected-session-leader-v1", "process %d is the leader of terminal session %d", p.PID, p.SID)
	}
	if in.IsAgentRoot {
		add("protected-agent-root-v1", "process is an agent session root; terminating it would end the user's session")
	}
	if in.SessionActive {
		add("protected-active-session-v1", "the owning agent session is still active")
	}
	if in.DescendantCount > 0 {
		add("protected-has-descendants-v1", "process has %d live descendant(s) that would be orphaned", in.DescendantCount)
	}
	if in.AttributionConfidence < adapters.ConfidencePolicyEligible {
		add("protected-uncertain-attribution-v1",
			"attribution confidence is %.2f, below the %.2f required for any policy to consider the process; unknown ownership is protected",
			in.AttributionConfidence, adapters.ConfidencePolicyEligible)
	}
	if !p.Detailed {
		add("protected-not-inspected-v1", "process was not inspected in detail, so nothing about it has been established")
	}

	base := strings.ToLower(p.Name())
	for _, class := range execClasses {
		for _, name := range class.names {
			if base != name {
				continue
			}
			add(class.id, "%s (executable %q)", class.reason, p.Name())
			break
		}
	}

	for _, ar := range in.AdapterRules {
		if matchAdapterRule(ar, p) {
			add(ar.ID, "%s", ar.Reason)
		}
	}

	return Result{Protected: len(rules) > 0, Rules: rules}
}

func matchAdapterRule(r adapters.ProtectionRule, p process.Process) bool {
	if r.Always {
		return true
	}
	base := path.Base(p.ExecPath)
	for _, n := range r.ExecNames {
		if base == n {
			return true
		}
	}
	if r.PathSegment != "" {
		want := strings.Split(strings.Trim(r.PathSegment, "/"), "/")
		segs := strings.Split(strings.Trim(p.ExecPath, "/"), "/")
		for i := 0; i+len(want) <= len(segs); i++ {
			match := true
			for j := range want {
				if segs[i+j] != want[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
