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

	s := &FakeProviderSession{
		id:     id,
		events: make(chan ProviderEvent, 128),
	}
	s.cond = sync.NewCond(&s.mu)
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
	cond   *sync.Cond
	closed bool

	receivedAudio [][]byte
	receivedTexts []string
	interrupts    int

	sendAudioErr error
	sendTextErr  error
	interruptErr error

	events chan ProviderEvent
}

// SendAudio satisfies ProviderSession.
func (s *FakeProviderSession) SendAudio(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendAudioErr != nil {
		return s.sendAudioErr
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	s.receivedAudio = append(s.receivedAudio, cp)
	if s.cond != nil {
		s.cond.Broadcast()
	}
	return nil
}

// SendText satisfies ProviderSession.
func (s *FakeProviderSession) SendText(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendTextErr != nil {
		return s.sendTextErr
	}
	s.receivedTexts = append(s.receivedTexts, text)
	return nil
}

// Interrupt satisfies ProviderSession.
func (s *FakeProviderSession) Interrupt(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupts++
	return s.interruptErr
}

// Events satisfies ProviderSession.
func (s *FakeProviderSession) Events() <-chan ProviderEvent {
	return s.events
}

// Close satisfies ProviderSession.
func (s *FakeProviderSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
		if s.cond != nil {
			s.cond.Broadcast()
		}
	}
	return nil
}

// IsClosed returns whether the session was closed.
func (s *FakeProviderSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// WaitForAudioCount blocks until at least count audio buffers are received or session is closed.
func (s *FakeProviderSession) WaitForAudioCount(count int) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.receivedAudio) < count && !s.closed {
		s.cond.Wait()
	}
	copied := make([][]byte, len(s.receivedAudio))
	for i, b := range s.receivedAudio {
		cp := make([]byte, len(b))
		copy(cp, b)
		copied[i] = cp
	}
	return copied
}

// ReceivedAudio returns all audio buffers received by the provider session.
func (s *FakeProviderSession) ReceivedAudio() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([][]byte, len(s.receivedAudio))
	for i, b := range s.receivedAudio {
		cp := make([]byte, len(b))
		copy(cp, b)
		copied[i] = cp
	}
	return copied
}

// ReceivedTexts returns all text inputs received by the provider session.
func (s *FakeProviderSession) ReceivedTexts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]string, len(s.receivedTexts))
	copy(copied, s.receivedTexts)
	return copied
}

// InterruptCount returns the number of times Interrupt was called.
func (s *FakeProviderSession) InterruptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interrupts
}

// SetSendAudioError configures an error to return on SendAudio.
func (s *FakeProviderSession) SetSendAudioError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendAudioErr = err
}

// SetSendTextError configures an error to return on SendText.
func (s *FakeProviderSession) SetSendTextError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendTextErr = err
}

// SetInterruptError configures an error to return on Interrupt.
func (s *FakeProviderSession) SetInterruptError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interruptErr = err
}

// EmitEvent enqueues a ProviderEvent to be consumed by the Session Engine.
func (s *FakeProviderSession) EmitEvent(ev ProviderEvent) {
	s.events <- ev
}

// EmitAudio delivers audio output from the provider.
func (s *FakeProviderSession) EmitAudio(turnID TurnID, data []byte) {
	s.EmitEvent(ProviderAudioEvent{TurnID: turnID, Data: data})
}

// EmitText delivers text output from the provider.
func (s *FakeProviderSession) EmitText(turnID TurnID, text string) {
	s.EmitEvent(ProviderTextEvent{TurnID: turnID, Text: text})
}

// EmitTurnStarted signals that the provider began a Turn.
func (s *FakeProviderSession) EmitTurnStarted(turnID TurnID) {
	s.EmitEvent(ProviderTurnStartedEvent{TurnID: turnID})
}

// EmitTurnCompleted signals that the provider completed a Turn.
func (s *FakeProviderSession) EmitTurnCompleted(turnID TurnID) {
	s.EmitEvent(ProviderTurnCompletedEvent{TurnID: turnID})
}

// EmitInterrupted signals that the provider detected interruption.
func (s *FakeProviderSession) EmitInterrupted(turnID TurnID) {
	s.EmitEvent(ProviderInterruptedEvent{TurnID: turnID})
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
