package session

import (
	"context"
	"sync"
)

// FakeVoiceProvider is a deterministic in-memory Voice Provider for tests.
type FakeVoiceProvider struct {
	mu       sync.Mutex
	startErr error
	sessions []*FakeProviderSession
}

// NewFakeVoiceProvider creates a new FakeVoiceProvider.
func NewFakeVoiceProvider() *FakeVoiceProvider {
	return &FakeVoiceProvider{}
}

// SetStartError configures an error to return on StartSession.
func (p *FakeVoiceProvider) SetStartError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startErr = err
}

// StartSession satisfies VoiceProvider.
func (p *FakeVoiceProvider) StartSession(_ context.Context, id SessionID) (ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.startErr != nil {
		return nil, p.startErr
	}

	s := &FakeProviderSession{id: id}
	p.sessions = append(p.sessions, s)
	return s, nil
}

// Sessions returns all started provider sessions.
func (p *FakeVoiceProvider) Sessions() []*FakeProviderSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := make([]*FakeProviderSession, len(p.sessions))
	copy(copied, p.sessions)
	return copied
}

// FakeProviderSession represents an in-memory session.
type FakeProviderSession struct {
	id     SessionID
	mu     sync.Mutex
	closed bool
}

// Close satisfies ProviderSession.
func (s *FakeProviderSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// IsClosed returns whether the session was closed.
func (s *FakeProviderSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// FakeAgentRuntime is a deterministic in-memory Agent Runtime for tests.
type FakeAgentRuntime struct {
	mu       sync.Mutex
	startErr error
	sessions []*FakeRuntimeSession
}

// NewFakeAgentRuntime creates a new FakeAgentRuntime.
func NewFakeAgentRuntime() *FakeAgentRuntime {
	return &FakeAgentRuntime{}
}

// SetStartError configures an error to return on StartSession.
func (r *FakeAgentRuntime) SetStartError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startErr = err
}

// StartSession satisfies AgentRuntime.
func (r *FakeAgentRuntime) StartSession(_ context.Context, id SessionID) (RuntimeSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.startErr != nil {
		return nil, r.startErr
	}

	s := &FakeRuntimeSession{id: id}
	r.sessions = append(r.sessions, s)
	return s, nil
}

// Sessions returns all started runtime sessions.
func (r *FakeAgentRuntime) Sessions() []*FakeRuntimeSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make([]*FakeRuntimeSession, len(r.sessions))
	copy(copied, r.sessions)
	return copied
}

// FakeRuntimeSession represents an in-memory runtime session.
type FakeRuntimeSession struct {
	id     SessionID
	mu     sync.Mutex
	closed bool
}

// Close satisfies RuntimeSession.
func (s *FakeRuntimeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// IsClosed returns whether the session was closed.
func (s *FakeRuntimeSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
