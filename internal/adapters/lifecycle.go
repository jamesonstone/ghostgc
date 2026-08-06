package adapters

import (
	"context"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// ProviderSessionState is an exact lifecycle state asserted by an agent's own
// durable metadata. Unknown provider data is omitted rather than guessed.
type ProviderSessionState string

const (
	ProviderSessionActive    ProviderSessionState = "active"
	ProviderSessionCompleted ProviderSessionState = "completed"
)

// KnownSession is the minimum committed identity an adapter may use to refresh
// lifecycle evidence after no process currently exposes the native ID.
type KnownSession struct {
	AgentID   string
	NativeID  string
	SessionID string
	RootKey   process.Key
	Metadata  SessionMetadata
}

// ProviderSession is one task-level session established by provider-owned
// lifecycle evidence and bound to an already identified agent host root.
type ProviderSession struct {
	AgentID   string
	NativeID  string
	SessionID string
	Root      AgentRoot
	State     ProviderSessionState
	StartedAt time.Time
	ChangedAt time.Time
	Metadata  SessionMetadata
	Evidence  []Evidence
}

// SessionLifecycleAdapter is optional. Process adapters that do not expose a
// separate task lifecycle continue to use only AgentAdapter.
type SessionLifecycleAdapter interface {
	DiscoverProviderSessions(context.Context, Graph) []ProviderSession
}
