package session

import "context"

// VoiceProvider defines the internal seam for realtime voice model providers.
type VoiceProvider interface {
	StartSession(ctx context.Context, id SessionID) (ProviderSession, error)
}

// ProviderSession represents an active session with a Voice Provider.
type ProviderSession interface {
	SendAudio(ctx context.Context, data []byte) error
	SendText(ctx context.Context, text string) error
	Interrupt(ctx context.Context) error
	Events() <-chan ProviderEvent
	Close() error
}

// ProviderEvent represents an event emitted by a Voice Provider session.
type ProviderEvent interface {
	isProviderEvent()
}

// ProviderAudioEvent delivers streaming audio data from the provider.
type ProviderAudioEvent struct {
	TurnID TurnID
	Data   []byte
}

func (ProviderAudioEvent) isProviderEvent() {}

// ProviderTextEvent delivers streaming text from the provider.
type ProviderTextEvent struct {
	TurnID TurnID
	Text   string
}

func (ProviderTextEvent) isProviderEvent() {}

// ProviderTurnStartedEvent signals that the provider began generating output for a Turn.
type ProviderTurnStartedEvent struct {
	TurnID TurnID
}

func (ProviderTurnStartedEvent) isProviderEvent() {}

// ProviderTurnCompletedEvent signals that the provider completed generation for a Turn.
type ProviderTurnCompletedEvent struct {
	TurnID TurnID
}

func (ProviderTurnCompletedEvent) isProviderEvent() {}

// ProviderInterruptedEvent signals that the provider detected user barge-in / speech.
type ProviderInterruptedEvent struct {
	TurnID TurnID
}

func (ProviderInterruptedEvent) isProviderEvent() {}

// AgentRuntime defines the internal seam for agentic runtimes.
type AgentRuntime interface {
	StartSession(ctx context.Context, id SessionID) (RuntimeSession, error)
}

// RuntimeSession represents an active session with an Agent Runtime.
type RuntimeSession interface {
	Close() error
}
