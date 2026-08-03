package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/adapters"
)

// DeriveSessionID produces a stable session identifier.
//
// When the agent exposes its own identifier that is used verbatim. Otherwise
// the identifier is derived from the root process key, which includes the
// start time, so a recycled PID can never collide with an earlier session.
func DeriveSessionID(root adapters.AgentRoot) string {
	if native := root.Metadata.SessionID; native != "" {
		return sanitize(native)
	}
	sum := sha256.Sum256([]byte(ID + "|" + root.Key.UID()))
	return hex.EncodeToString(sum[:8])
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

func segments(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(strings.Trim(p, "/"), "/")
}

func containsSegment(segs []string, want string) bool {
	for _, s := range segs {
		if s == want {
			return true
		}
	}
	return false
}

func containsSegmentSuffix(segs []string, suffix string) bool {
	for _, s := range segs {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func hasSegmentPair(segs []string, first, second string) bool {
	for i := 0; i+1 < len(segs); i++ {
		if segs[i] == first && segs[i+1] == second {
			return true
		}
	}
	return false
}
