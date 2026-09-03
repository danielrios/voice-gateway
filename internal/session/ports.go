package session

import "context"

// VoiceProvider defines the internal seam for realtime voice model providers.
type VoiceProvider interface {
	StartSession(ctx context.Context, id SessionID) (ProviderSession, error)
}

// ProviderSession represents an active session with a Voice Provider.
type ProviderSession interface {
	Close() error
}

// AgentRuntime defines the internal seam for agentic runtimes.
type AgentRuntime interface {
	StartSession(ctx context.Context, id SessionID) (RuntimeSession, error)
}

// RuntimeSession represents an active session with an Agent Runtime.
type RuntimeSession interface {
	Close() error
}
