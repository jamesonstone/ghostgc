#!/bin/bash
# Build a controlled process tree that looks like a coding-agent session, so the
# real platform collector can be exercised against something whose shape is
# known in advance.
#
# The unit and integration tests use a scripted fake platform. This fixture
# tests the part a fake cannot: that sysctl, libproc and the tree builder agree
# with reality on this machine.
#
#   fixtures/fixture-agent.sh start     create the tree, print the root pid
#   fixtures/fixture-agent.sh orphan    kill only the root, leaving survivors
#   fixtures/fixture-agent.sh stop      remove everything this fixture created
#
# The fixture only ever signals processes it started itself, whose pids it
# recorded at creation. ghostgc does not signal anything, in this delivery
# phase or in this script.

set -euo pipefail

STATE_DIR="${TMPDIR:-/tmp}/ghostgc-fixture"
PID_FILE="$STATE_DIR/pids"
ROOT_PID_FILE="$STATE_DIR/root"
BIN="$STATE_DIR/bin/codex"

usage() {
	sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

make_fake_agent() {
	mkdir -p "$STATE_DIR/bin" "$STATE_DIR/repo/.git"
	# Built, not copied, and built to a file named "codex": the kernel records
	# that path as the process image, so proc_pidpath reports a basename of
	# "codex" exactly as the real CLI does. A shebang script would report the
	# interpreter instead, and a copy of a system binary is SIGKILLed on Apple
	# Silicon because copying invalidates its code signature.
	local src
	src="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/fake-agent"
	go build -o "$BIN" "$src"
	# The detached helper is built under a different name on purpose. If it
	# were another copy of "codex" it would be detected as an agent in its own
	# right, and the fixture would stop exercising the case it exists for: a
	# process that is only linked to the session by the environment it
	# inherited.
	go build -o "$STATE_DIR/bin/fixture-helper" "$src"
}

start() {
	if [[ -f "$ROOT_PID_FILE" ]] && kill -0 "$(cat "$ROOT_PID_FILE")" 2>/dev/null; then
		echo "fixture already running with root pid $(cat "$ROOT_PID_FILE")" >&2
		exit 1
	fi
	make_fake_agent
	: >"$PID_FILE"

	# The session root, with three descendants:
	#   active-child   busy in a loop
	#   idle-child     sleeping
	#   detached-child outlives an intermediate shell that exits immediately,
	#                  so the kernel reparents it to launchd
	cd "$STATE_DIR/repo"
	# The agent's own session identifier. Children inherit it through the
	# environment, which is how a process that has since been reparented can
	# still be traced back to the session that started it.
	export CODEX_SESSION_ID="fixture-$$-$(date +%s)"
	echo "$CODEX_SESSION_ID" >"$STATE_DIR/session-id"
	"$BIN" "$STATE_DIR/repo" >>"$STATE_DIR/fixture.out" 2>&1 &
	root=$!
	echo "$root" >"$ROOT_PID_FILE"
	echo "root $root" >>"$PID_FILE"

	sleep 2
	# Record the descendants the fake agent reported, so teardown only ever
	# signals processes this fixture created.
	awk '/^fixture /{print $2, $3}' "$STATE_DIR/fixture.out" >>"$PID_FILE" 2>/dev/null || true
	echo "fixture root pid: $root"
	echo "repository:       $STATE_DIR/repo"
	echo "session id:       $CODEX_SESSION_ID"
	echo
	echo "Wait one scan interval, then:"
	echo "  ghostgc sessions"
	echo "  ghostgc explain $root"
}

orphan() {
	[[ -f "$ROOT_PID_FILE" ]] || { echo "no fixture is running" >&2; exit 1; }
	root=$(cat "$ROOT_PID_FILE")
	echo "ending the fixture session root $root; its descendants survive"
	kill -TERM "$root" 2>/dev/null || true
	rm -f "$ROOT_PID_FILE"
	echo
	echo "After the next scan the session should report state=completed and the"
	echo "survivors should keep their recorded ownership:"
	echo "  ghostgc sessions"
	echo "  ghostgc processes --all"
}

stop() {
	if [[ -f "$PID_FILE" ]]; then
		while read -r _label pid; do
			[[ -n "${pid:-}" ]] && kill -TERM "$pid" 2>/dev/null || true
		done <"$PID_FILE"
	fi
	rm -rf "$STATE_DIR"
	echo "fixture removed"
}

case "${1:-}" in
	start) start ;;
	orphan) orphan ;;
	stop) stop ;;
	*) usage ;;
esac
