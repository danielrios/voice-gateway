package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// SessionEngine manages the lifecycle of Voice Sessions.
type SessionEngine interface {
	Open(ctx context.Context, req OpenRequest) (SessionHandle, error)
}

// SessionHandle provides an interface to interact with an active or dormant Voice Session.
type SessionHandle interface {
	ID() SessionID
	Attach(ctx context.Context) (ClientLink, error)
	End(ctx context.Context) error
}

// ClientLink connects a Voice Client transport to an active Voice Session.
type ClientLink interface {
	Send(ctx context.Context, input SessionInput) error
	Events() <-chan SessionOutput
	Detach() error
}

type engine struct {
	provider VoiceProvider
	runtime  AgentRuntime
}

// NewEngine creates a new SessionEngine with the provided VoiceProvider and AgentRuntime ports.
func NewEngine(provider VoiceProvider, runtime AgentRuntime) SessionEngine {
	return &engine{
		provider: provider,
		runtime:  runtime,
	}
}

func (e *engine) Open(ctx context.Context, req OpenRequest) (SessionHandle, error) {
	id := req.Resume
	if id == "" {
		id = SessionID(generateSessionID())
	}
	sessID := id

	var (
		pSession ProviderSession
		rSession RuntimeSession
		err      error
	)
	if e.provider != nil {
		pSession, err = e.provider.StartSession(ctx, sessID)
		if err != nil {
			return nil, err
		}
	}
	if e.runtime != nil {
		rSession, err = e.runtime.StartSession(ctx, sessID)
		if err != nil {
			if pSession != nil {
				_ = pSession.Close()
			}
			return nil, err
		}
	}

	return newHandle(sessID, pSession, rSession), nil
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sess_%s", hex.EncodeToString(b))
}
