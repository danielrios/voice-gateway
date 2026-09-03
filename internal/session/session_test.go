package session_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/danielrios/voice-gateway/internal/session"
)

func assertEventsClosed(t *testing.T, ch <-chan session.SessionOutput) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("expected events channel to be closed, but received event: %v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for events channel to close")
	}
}

func TestOpenSessionReturnsSessionIdentity(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}
	if handle == nil {
		t.Fatal("expected non-nil SessionHandle")
	}

	sessionID := handle.ID()
	if string(sessionID) == "" {
		t.Fatal("expected non-empty SessionID")
	}
}

func TestOpenSessionFailsOnEntropyExhaustion(t *testing.T) {
	restore := session.SetRandReaderForTesting(errReader{})
	defer restore()

	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err == nil {
		t.Fatal("expected error on entropy failure, got nil")
	}
	if handle != nil {
		t.Fatal("expected nil handle on entropy failure")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy exhausted")
}

func TestAttachAndDetachClientLinkWithoutEndingSession(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching client link: %v", err)
	}
	if link == nil {
		t.Fatal("expected non-nil ClientLink")
	}

	if err := link.Detach(); err != nil {
		t.Fatalf("unexpected error detaching client link: %v", err)
	}

	// Verify events channel is closed upon detach using timed assertion
	assertEventsClosed(t, link.Events())

	// Sending on a detached link should fail with ErrLinkDetached
	if err := link.Send(ctx, session.ClientTextInput{Text: "hello"}); err != session.ErrLinkDetached {
		t.Fatalf("expected ErrLinkDetached, got %v", err)
	}

	// Voice Session should still be open; attaching another link should succeed
	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("expected session to remain open and allow attach after detach, got %v", err)
	}
	if link2 == nil {
		t.Fatal("expected non-nil ClientLink for second attach")
	}
}

func TestDetachIsIdempotent(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching: %v", err)
	}

	// First detach
	if err := link.Detach(); err != nil {
		t.Fatalf("unexpected error on first detach: %v", err)
	}

	// Repeated detach calls should be safe and return nil
	for i := 0; i < 5; i++ {
		if err := link.Detach(); err != nil {
			t.Fatalf("iteration %d: expected nil on repeated detach, got %v", i, err)
		}
	}

	if err := link.Send(ctx, session.ClientTextInput{Text: "test"}); err != session.ErrLinkDetached {
		t.Fatalf("expected ErrLinkDetached, got %v", err)
	}
}

func TestNewClientLinkSupersedesOldLink(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	link1, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching first link: %v", err)
	}

	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching second link: %v", err)
	}

	// Verify link1 events channel is closed upon supersession
	assertEventsClosed(t, link1.Events())

	// link1 Send should return ErrLinkSuperseded
	if err := link1.Send(ctx, session.ClientTextInput{Text: "stale"}); err != session.ErrLinkSuperseded {
		t.Fatalf("expected ErrLinkSuperseded from link1, got %v", err)
	}

	// link2 should be active and able to send
	if err := link2.Send(ctx, session.ClientTextInput{Text: "current"}); err != nil {
		t.Fatalf("expected link2 Send to succeed, got %v", err)
	}
}

func TestReattachRetainsSessionIdentity(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	sessionID := handle.ID()

	link1, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching first link: %v", err)
	}

	if err := link1.Detach(); err != nil {
		t.Fatalf("unexpected error detaching: %v", err)
	}

	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching second link: %v", err)
	}

	if handle.ID() != sessionID {
		t.Fatalf("expected session ID %q to be retained, got %q", sessionID, handle.ID())
	}

	if err := link2.Send(ctx, session.ClientTextInput{Text: "after reattach"}); err != nil {
		t.Fatalf("expected link2 send to succeed, got %v", err)
	}

	if err := link1.Send(ctx, session.ClientTextInput{Text: "on old link"}); err != session.ErrLinkDetached {
		t.Fatalf("expected old detached link to remain detached, got %v", err)
	}
}

func TestExplicitEndLifecycle(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching link: %v", err)
	}

	// Explicit End should invalidate active link
	if err := handle.End(ctx); err != nil {
		t.Fatalf("unexpected error on handle.End: %v", err)
	}

	// Events channel should be closed
	assertEventsClosed(t, link.Events())

	// Send on invalidated link should fail
	if err := link.Send(ctx, session.ClientTextInput{Text: "after end"}); err == nil {
		t.Fatal("expected error sending on invalidated link after End")
	}

	// End should be safe to repeat
	for i := 0; i < 5; i++ {
		if err := handle.End(ctx); err != nil {
			t.Fatalf("iteration %d: expected nil on repeated handle.End, got %v", i, err)
		}
	}

	// Subsequent attach must be rejected with ErrSessionEnded
	if _, err := handle.Attach(ctx); err != session.ErrSessionEnded {
		t.Fatalf("expected ErrSessionEnded on attach after End, got %v", err)
	}
}

func TestSendDoesNotDeadlockWhenRacingWithEnd(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected error attaching: %v", err)
	}

	done := make(chan struct{})
	go func() {
		// Concurrent Send with non-canceled context
		_ = link.Send(context.Background(), session.ClientTextInput{Text: "racing send"})
		close(done)
	}()

	_ = handle.End(context.Background())

	select {
	case <-done:
		// success: Send completed without deadlocking
	case <-time.After(1 * time.Second):
		t.Fatal("Send deadlocked when racing with End")
	}
}

func TestDrainingBufferedCommandsOnSessionEnd(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := handle.Attach(context.Background())
			if err == nil && link != nil {
				_ = link.Send(context.Background(), session.ClientTextInput{Text: "msg"})
			}
		}()
	}

	_ = handle.End(context.Background())

	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// all callers unblocked cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("callers blocked indefinitely waiting on buffered commands during End")
	}
}

func TestProviderAndRuntimeSeamLifecycle(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	pSessions := provider.Sessions()
	if len(pSessions) != 1 {
		t.Fatalf("expected 1 provider session started, got %d", len(pSessions))
	}
	rSessions := runtime.Sessions()
	if len(rSessions) != 1 {
		t.Fatalf("expected 1 runtime session started, got %d", len(rSessions))
	}

	if pSessions[0].IsClosed() {
		t.Fatal("expected provider session to be active")
	}
	if rSessions[0].IsClosed() {
		t.Fatal("expected runtime session to be active")
	}

	if err := handle.End(ctx); err != nil {
		t.Fatalf("unexpected error ending session: %v", err)
	}

	if !pSessions[0].IsClosed() {
		t.Fatal("expected provider session to be closed upon handle.End")
	}
	if !rSessions[0].IsClosed() {
		t.Fatal("expected runtime session to be closed upon handle.End")
	}
}

func TestConcurrentAttachDetachEnd(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := handle.Attach(ctx)
			if err != nil {
				return
			}
			_ = link.Send(ctx, session.ClientTextInput{Text: "concurrent message"})
			_ = link.Detach()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = handle.End(ctx)
	}()

	wg.Wait()
}

func TestEndContextCancellationAllowsSubsequentEnd(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected error opening session: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	// Calling End with canceled context should return error
	if err := handle.End(canceledCtx); err == nil {
		t.Fatal("expected error with pre-canceled context")
	}

	// Subsequent End with valid context should succeed and cleanly terminate session
	if err := handle.End(ctx); err != nil {
		t.Fatalf("expected subsequent End to succeed, got %v", err)
	}

	if _, err := handle.Attach(ctx); err != session.ErrSessionEnded {
		t.Fatalf("expected ErrSessionEnded after successful End, got %v", err)
	}
}

func TestLinkStateTransitionsTable(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(handle session.SessionHandle) (session.ClientLink, error)
		expectSendErr error
	}{
		{
			name: "active link allows send",
			setup: func(h session.SessionHandle) (session.ClientLink, error) {
				return h.Attach(context.Background())
			},
			expectSendErr: nil,
		},
		{
			name: "detached link returns ErrLinkDetached",
			setup: func(h session.SessionHandle) (session.ClientLink, error) {
				l, err := h.Attach(context.Background())
				if err != nil {
					return nil, err
				}
				if err := l.Detach(); err != nil {
					return nil, err
				}
				return l, nil
			},
			expectSendErr: session.ErrLinkDetached,
		},
		{
			name: "superseded link returns ErrLinkSuperseded",
			setup: func(h session.SessionHandle) (session.ClientLink, error) {
				l1, err := h.Attach(context.Background())
				if err != nil {
					return nil, err
				}
				_, err = h.Attach(context.Background())
				if err != nil {
					return nil, err
				}
				return l1, nil
			},
			expectSendErr: session.ErrLinkSuperseded,
		},
		{
			name: "link terminated by End returns error",
			setup: func(h session.SessionHandle) (session.ClientLink, error) {
				l, err := h.Attach(context.Background())
				if err != nil {
					return nil, err
				}
				if err := h.End(context.Background()); err != nil {
					return nil, err
				}
				return l, nil
			},
			expectSendErr: session.ErrLinkTerminal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			provider := session.NewFakeVoiceProvider()
			runtime := session.NewFakeAgentRuntime()
			engine := session.NewEngine(provider, runtime)

			h, err := engine.Open(ctx, session.OpenRequest{})
			if err != nil {
				t.Fatalf("unexpected open error: %v", err)
			}
			link, err := tc.setup(h)
			if err != nil {
				t.Fatalf("unexpected setup error: %v", err)
			}

			err = link.Send(ctx, session.ClientTextInput{Text: "test"})
			if tc.expectSendErr == nil {
				if err != nil {
					t.Fatalf("expected nil send error, got %v", err)
				}
			} else if err != tc.expectSendErr {
				t.Fatalf("expected error %v, got %v", tc.expectSendErr, err)
			}
		})
	}
}

func TestOpenPropagatesProviderAndRuntimeErrors(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("boom")

	t.Run("provider start failure", func(t *testing.T) {
		provider := session.NewFakeVoiceProvider()
		provider.SetStartError(testErr)
		runtime := session.NewFakeAgentRuntime()
		engine := session.NewEngine(provider, runtime)

		handle, err := engine.Open(ctx, session.OpenRequest{})
		if !errors.Is(err, testErr) {
			t.Fatalf("expected error %v, got %v", testErr, err)
		}
		if handle != nil {
			t.Fatal("expected nil handle on provider open error")
		}
		if len(runtime.Sessions()) != 0 {
			t.Fatal("runtime session should not be started if provider fails")
		}
	})

	t.Run("runtime start failure cleans up provider", func(t *testing.T) {
		provider := session.NewFakeVoiceProvider()
		runtime := session.NewFakeAgentRuntime()
		runtime.SetStartError(testErr)
		engine := session.NewEngine(provider, runtime)

		handle, err := engine.Open(ctx, session.OpenRequest{})
		if !errors.Is(err, testErr) {
			t.Fatalf("expected error %v, got %v", testErr, err)
		}
		if handle != nil {
			t.Fatal("expected nil handle on runtime open error")
		}
		pSessions := provider.Sessions()
		if len(pSessions) != 1 {
			t.Fatalf("expected 1 provider session attempted, got %d", len(pSessions))
		}
		if !pSessions[0].IsClosed() {
			t.Fatal("provider session should be closed when runtime start fails")
		}
	})
}

func readEventWithTimeout(t *testing.T, ch <-chan session.SessionOutput) session.SessionOutput {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("expected session output, but channel was closed")
		}
		return ev
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for session output")
		return nil
	}
}

func TestClientAudioAndTextInputAcceptedWithoutProviderTypes(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pcmAudio := []byte{0x01, 0x02, 0x03, 0x04}
	if err := link.Send(ctx, session.ClientAudioInput{Data: pcmAudio}); err != nil {
		t.Fatalf("unexpected error sending audio: %v", err)
	}

	text := "transcribed user speech"
	if err := link.Send(ctx, session.ClientTextInput{Text: text}); err != nil {
		t.Fatalf("unexpected error sending text: %v", err)
	}

	pSessions := provider.Sessions()
	if len(pSessions) != 1 {
		t.Fatalf("expected 1 provider session, got %d", len(pSessions))
	}
	pSess := pSessions[0]

	audios := pSess.ReceivedAudio()
	if len(audios) != 1 || !bytes.Equal(audios[0], pcmAudio) {
		t.Fatalf("expected provider to receive exact pcm audio %v, got %v", pcmAudio, audios)
	}

	texts := pSess.ReceivedTexts()
	if len(texts) != 1 || texts[0] != text {
		t.Fatalf("expected provider to receive exact text %q, got %v", text, texts)
	}
}

func TestSessionOutputStreamsProviderAudioTextAndControlFacts(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Provider emits TurnStarted, Text, Audio, and TurnCompleted
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)
	pSess.EmitText(turnID, "hello from provider")
	pSess.EmitAudio(turnID, []byte{0x0a, 0x0b, 0x0c})
	pSess.EmitTurnCompleted(turnID)

	ev1 := readEventWithTimeout(t, link.Events())
	start, ok := ev1.(session.TurnStartedOutput)
	if !ok || start.TurnID != turnID {
		t.Fatalf("expected TurnStartedOutput with %q, got %#v", turnID, ev1)
	}

	ev2 := readEventWithTimeout(t, link.Events())
	txt, ok := ev2.(session.TextOutput)
	if !ok || txt.TurnID != turnID || txt.Text != "hello from provider" {
		t.Fatalf("expected TextOutput with %q, got %#v", turnID, ev2)
	}

	ev3 := readEventWithTimeout(t, link.Events())
	aud, ok := ev3.(session.AudioOutput)
	if !ok || aud.TurnID != turnID || !bytes.Equal(aud.Data, []byte{0x0a, 0x0b, 0x0c}) {
		t.Fatalf("expected AudioOutput with %q, got %#v", turnID, ev3)
	}

	ev4 := readEventWithTimeout(t, link.Events())
	comp, ok := ev4.(session.TurnCompletedOutput)
	if !ok || comp.TurnID != turnID {
		t.Fatalf("expected TurnCompletedOutput with %q, got %#v", turnID, ev4)
	}
}

func TestUserInterruptionInvalidatesQueuedPlayback(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Start Turn 1
	if err := link.Send(ctx, session.ClientTextInput{Text: "start turn 1"}); err != nil {
		t.Fatalf("unexpected send error: %v", err)
	}
	ev1 := readEventWithTimeout(t, link.Events())
	start1, ok := ev1.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", ev1)
	}
	turn1ID := start1.TurnID

	// Provider emits audio chunks for Turn 1 while client is not reading
	pSess.EmitAudio(turn1ID, []byte("chunk-1"))
	pSess.EmitAudio(turn1ID, []byte("chunk-2"))
	pSess.EmitAudio(turn1ID, []byte("chunk-3"))
	pSess.EmitAudio(turn1ID, []byte("chunk-4"))

	// Client reads only chunk-1
	evChunk1 := readEventWithTimeout(t, link.Events())
	aud1, ok := evChunk1.(session.AudioOutput)
	if !ok || string(aud1.Data) != "chunk-1" {
		t.Fatalf("expected chunk-1 AudioOutput, got %#v", evChunk1)
	}

	// User interrupts by sending new input for Turn 2
	if err := link.Send(ctx, session.ClientTextInput{Text: "interrupt with turn 2"}); err != nil {
		t.Fatalf("unexpected send error on interruption: %v", err)
	}

	// Provider emits audio for Turn 2
	turn2ID := session.TurnID("turn_2")
	pSess.EmitAudio(turn2ID, []byte("turn2-audio"))

	// Next event MUST be TurnInterruptedOutput for turn 1
	evInterrupted := readEventWithTimeout(t, link.Events())
	interrupted, ok := evInterrupted.(session.TurnInterruptedOutput)
	if !ok || interrupted.TurnID != turn1ID {
		t.Fatalf("expected TurnInterruptedOutput for %q, got %#v", turn1ID, evInterrupted)
	}

	// Next event MUST be TurnStartedOutput for turn 2
	evStart2 := readEventWithTimeout(t, link.Events())
	start2, ok := evStart2.(session.TurnStartedOutput)
	if !ok || start2.TurnID != turn2ID {
		t.Fatalf("expected TurnStartedOutput for %q, got %#v", turn2ID, evStart2)
	}

	// Next event MUST be turn 2 audio; chunks 2, 3, 4 of turn 1 must NEVER cross link.Events()!
	evTurn2Audio := readEventWithTimeout(t, link.Events())
	aud2, ok := evTurn2Audio.(session.AudioOutput)
	if !ok || aud2.TurnID != turn2ID || string(aud2.Data) != "turn2-audio" {
		t.Fatalf("expected AudioOutput for turn 2, got %#v", evTurn2Audio)
	}

	// Verify provider received Interrupt call
	if pSess.InterruptCount() < 1 {
		t.Fatalf("expected provider Interrupt to be called, got count %d", pSess.InterruptCount())
	}
}

func TestStaleProviderCompletionDoesNotCompleteCurrentTurn(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Start Turn 1
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 1"})
	ev1 := readEventWithTimeout(t, link.Events())
	start1, ok := ev1.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", ev1)
	}
	turn1ID := start1.TurnID

	// Provider begins generating for turn 1
	pSess.EmitAudio(turn1ID, []byte("chunk-turn1"))
	_ = readEventWithTimeout(t, link.Events()) // read chunk-turn1

	// User interrupts with Turn 2
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 2"})
	evInt := readEventWithTimeout(t, link.Events())
	if _, ok := evInt.(session.TurnInterruptedOutput); !ok {
		t.Fatalf("expected TurnInterruptedOutput, got %#v", evInt)
	}
	evStart2 := readEventWithTimeout(t, link.Events())
	start2, ok := evStart2.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput for turn 2, got %#v", evStart2)
	}
	turn2ID := start2.TurnID

	// Provider emits a delayed completion from superseded turn 1
	pSess.EmitTurnCompleted(turn1ID)

	// Provider emits audio for turn 2
	pSess.EmitAudio(turn2ID, []byte("chunk-turn2"))
	evTurn2Audio := readEventWithTimeout(t, link.Events())
	aud2, ok := evTurn2Audio.(session.AudioOutput)
	if !ok || aud2.TurnID != turn2ID {
		t.Fatalf("expected turn 2 audio, but received %#v (stale completion prematurely completed turn 2)", evTurn2Audio)
	}

	// Now provider completes turn 2
	pSess.EmitTurnCompleted(turn2ID)
	evTurn2Comp := readEventWithTimeout(t, link.Events())
	comp2, ok := evTurn2Comp.(session.TurnCompletedOutput)
	if !ok || comp2.TurnID != turn2ID {
		t.Fatalf("expected TurnCompletedOutput for turn 2, got %#v", evTurn2Comp)
	}
}

func TestStaleProviderAudioAfterInterruptionIsDiscarded(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Start Turn 1
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 1"})
	ev1 := readEventWithTimeout(t, link.Events())
	turn1ID := ev1.(session.TurnStartedOutput).TurnID

	// Provider starts generating for turn 1
	pSess.EmitAudio(turn1ID, []byte("first-turn-audio"))
	_ = readEventWithTimeout(t, link.Events())

	// Interrupt turn 1 with turn 2
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 2"})
	_ = readEventWithTimeout(t, link.Events()) // TurnInterruptedOutput
	evStart2 := readEventWithTimeout(t, link.Events())
	turn2ID := evStart2.(session.TurnStartedOutput).TurnID

	// Late-arriving audio for superseded turn 1 from provider
	pSess.EmitAudio(turn1ID, []byte("stale-audio-turn1"))

	// Fresh audio for turn 2
	pSess.EmitAudio(turn2ID, []byte("fresh-audio-turn2"))

	evAudio := readEventWithTimeout(t, link.Events())
	aud, ok := evAudio.(session.AudioOutput)
	if !ok || aud.TurnID != turn2ID || string(aud.Data) != "fresh-audio-turn2" {
		t.Fatalf("expected fresh audio for turn 2, got %#v", evAudio)
	}
}

func TestExplicitClientInterruptedInput(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	_ = link.Send(ctx, session.ClientTextInput{Text: "speak to me"})
	ev1 := readEventWithTimeout(t, link.Events())
	turnID := ev1.(session.TurnStartedOutput).TurnID

	pSess.EmitAudio(turnID, []byte("audio-part-1"))
	pSess.EmitAudio(turnID, []byte("audio-part-2"))

	_ = readEventWithTimeout(t, link.Events()) // audio-part-1

	// Explicit client interruption
	if err := link.Send(ctx, session.ClientInterruptedInput{}); err != nil {
		t.Fatalf("unexpected error on ClientInterruptedInput: %v", err)
	}

	evInt := readEventWithTimeout(t, link.Events())
	interrupted, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || interrupted.TurnID != turnID {
		t.Fatalf("expected TurnInterruptedOutput, got %#v", evInt)
	}

	if pSess.InterruptCount() != 1 {
		t.Fatalf("expected 1 Interrupt on provider, got %d", pSess.InterruptCount())
	}
}

func TestProviderInterruptedEventBargeIn(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)
	_ = readEventWithTimeout(t, link.Events()) // TurnStartedOutput

	pSess.EmitAudio(turnID, []byte("audio-chunk"))
	_ = readEventWithTimeout(t, link.Events()) // AudioOutput

	// Provider VAD detects barge-in
	pSess.EmitInterrupted(turnID)

	evInt := readEventWithTimeout(t, link.Events())
	interrupted, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || interrupted.TurnID != turnID {
		t.Fatalf("expected TurnInterruptedOutput for %q, got %#v", turnID, evInt)
	}
}

func TestBoundedMediaQueueDropPolicy(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Set queue bound to 3
	session.SetMediaQueueCapacityForTesting(link, 3)

	pSess := provider.Sessions()[0]
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)

	evStart := readEventWithTimeout(t, link.Events())
	if _, ok := evStart.(session.TurnStartedOutput); !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", evStart)
	}

	// Emit 8 audio chunks without client reading
	for i := 0; i < 8; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("chunk-%d", i)))
	}

	// Wait for the 5 oldest chunks to be dropped under capacity 3
	session.WaitForDroppedMediaCountForTesting(link, 5)

	// The remaining 3 chunks in queue must be the freshest ones: chunk-5, chunk-6, chunk-7
	for expectedIdx := 5; expectedIdx < 8; expectedIdx++ {
		ev := readEventWithTimeout(t, link.Events())
		aud, ok := ev.(session.AudioOutput)
		if !ok {
			t.Fatalf("expected AudioOutput, got %#v", ev)
		}
		expectedStr := fmt.Sprintf("chunk-%d", expectedIdx)
		if string(aud.Data) != expectedStr {
			t.Fatalf("expected freshest chunk %q, got %q", expectedStr, string(aud.Data))
		}
	}

	// Because capacity is 3 and 8 were sent, exactly 5 chunks were dropped
	dropped := session.GetDroppedMediaCountForTesting(link)
	if dropped != 5 {
		t.Fatalf("expected 5 dropped chunks, got %d", dropped)
	}
}

func TestControlEventsNeverDroppedUnderMediaBackpressure(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Set queue bound to 2
	session.SetMediaQueueCapacityForTesting(link, 2)

	pSess := provider.Sessions()[0]
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)

	// Flood queue with 20 audio chunks without client reading
	for i := 0; i < 20; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("flood-%d", i)))
	}

	// Wait for the 18 oldest chunks to be dropped under capacity 2
	session.WaitForDroppedMediaCountForTesting(link, 18)

	// Now emit TurnCompleted
	pSess.EmitTurnCompleted(turnID)

	// Read events: first must be TurnStartedOutput
	evStart := readEventWithTimeout(t, link.Events())
	if _, ok := evStart.(session.TurnStartedOutput); !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", evStart)
	}

	// Next must be bounded audio chunks (at most 2)
	readAudioCount := 0
	for i := 0; i < 2; i++ {
		ev := readEventWithTimeout(t, link.Events())
		if _, ok := ev.(session.AudioOutput); ok {
			readAudioCount++
		} else if comp, ok := ev.(session.TurnCompletedOutput); ok {
			if comp.TurnID != turnID {
				t.Fatalf("expected TurnCompletedOutput for %q, got %#v", turnID, comp)
			}
			return // Completed was received safely!
		}
	}

	// Next must be TurnCompletedOutput (it was never dropped!)
	evComp := readEventWithTimeout(t, link.Events())
	comp, ok := evComp.(session.TurnCompletedOutput)
	if !ok || comp.TurnID != turnID {
		t.Fatalf("expected TurnCompletedOutput for %q, got %#v", turnID, evComp)
	}
}

func TestContinuousUserAudioStreamingDoesNotInterruptSameTurn(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Send 3 audio chunks in the same user turn
	for i := 1; i <= 3; i++ {
		if err := link.Send(ctx, session.ClientAudioInput{Data: []byte(fmt.Sprintf("pcm-%d", i))}); err != nil {
			t.Fatalf("chunk %d send error: %v", i, err)
		}
	}

	// Only 1 TurnStartedOutput should be emitted
	ev1 := readEventWithTimeout(t, link.Events())
	start, ok := ev1.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", ev1)
	}

	// Provider should have received all 3 chunks
	audios := pSess.WaitForAudioCount(3)
	if len(audios) != 3 {
		t.Fatalf("expected 3 audio chunks received by provider, got %d", len(audios))
	}

	// Provider completes the turn
	pSess.EmitTurnCompleted(start.TurnID)
	evComp := readEventWithTimeout(t, link.Events())
	if comp, ok := evComp.(session.TurnCompletedOutput); !ok || comp.TurnID != start.TurnID {
		t.Fatalf("expected TurnCompletedOutput for %q, got %#v", start.TurnID, evComp)
	}
}

func TestSupersededAndDetachedLinkCannotSendOrReceiveMedia(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	// 1. Detached link verification
	link1, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}
	if err := link1.Detach(); err != nil {
		t.Fatalf("unexpected detach error: %v", err)
	}

	assertEventsClosed(t, link1.Events())

	if err := link1.Send(ctx, session.ClientAudioInput{Data: []byte("audio-on-detached")}); err != session.ErrLinkDetached {
		t.Fatalf("expected ErrLinkDetached sending audio on detached link, got %v", err)
	}

	// 2. Superseded link verification
	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	link3, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	assertEventsClosed(t, link2.Events())

	if err := link2.Send(ctx, session.ClientAudioInput{Data: []byte("audio-on-superseded")}); err != session.ErrLinkSuperseded {
		t.Fatalf("expected ErrLinkSuperseded sending audio on superseded link, got %v", err)
	}

	// Active link3 can send and receive normally
	if err := link3.Send(ctx, session.ClientAudioInput{Data: []byte("active-link-audio")}); err != nil {
		t.Fatalf("unexpected error sending on active link3: %v", err)
	}

	evStart := readEventWithTimeout(t, link3.Events())
	if _, ok := evStart.(session.TurnStartedOutput); !ok {
		t.Fatalf("expected TurnStartedOutput on link3, got %#v", evStart)
	}
}

func TestAudioStreamingOverMultipleTurns(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	for turnNum := 1; turnNum <= 3; turnNum++ {
		// Client sends input
		if err := link.Send(ctx, session.ClientTextInput{Text: fmt.Sprintf("hello turn %d", turnNum)}); err != nil {
			t.Fatalf("turn %d send error: %v", turnNum, err)
		}

		evStart := readEventWithTimeout(t, link.Events())
		start, ok := evStart.(session.TurnStartedOutput)
		if !ok {
			t.Fatalf("turn %d: expected TurnStartedOutput, got %#v", turnNum, evStart)
		}
		expectedTurnID := session.TurnID(fmt.Sprintf("turn_%d", turnNum))
		if start.TurnID != expectedTurnID {
			t.Fatalf("turn %d: expected %q, got %q", turnNum, expectedTurnID, start.TurnID)
		}

		// Provider streams audio
		pSess.EmitAudio(start.TurnID, []byte(fmt.Sprintf("resp-%d", turnNum)))
		evAudio := readEventWithTimeout(t, link.Events())
		aud, ok := evAudio.(session.AudioOutput)
		if !ok || aud.TurnID != start.TurnID || string(aud.Data) != fmt.Sprintf("resp-%d", turnNum) {
			t.Fatalf("turn %d: expected AudioOutput, got %#v", turnNum, evAudio)
		}

		// Provider completes turn
		pSess.EmitTurnCompleted(start.TurnID)
		evComp := readEventWithTimeout(t, link.Events())
		comp, ok := evComp.(session.TurnCompletedOutput)
		if !ok || comp.TurnID != start.TurnID {
			t.Fatalf("turn %d: expected TurnCompletedOutput, got %#v", turnNum, evComp)
		}
	}
}

func TestConcurrentAudioStreamingAndInterruption(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	var wg sync.WaitGroup

	// Goroutine streaming audio
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			_ = link.Send(ctx, session.ClientAudioInput{Data: []byte(fmt.Sprintf("audio-%d", i))})
		}
	}()

	// Goroutine sending interruption
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = link.Send(ctx, session.ClientInterruptedInput{})
		}
	}()

	// Reader goroutine reading from Events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			select {
			case _, ok := <-link.Events():
				if !ok {
					return
				}
			case <-time.After(500 * time.Millisecond):
				return
			}
		}
	}()

	wg.Wait()
	_ = handle.End(ctx)
}

func TestProviderSendTextErrorPropagated(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]
	expectedErr := errors.New("provider text failure")
	pSess.SetSendTextError(expectedErr)

	err = link.Send(ctx, session.ClientTextInput{Text: "fail text"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestStaleProviderInterruptedIgnored(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Start turn 1
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 1"})
	ev1 := readEventWithTimeout(t, link.Events())
	turn1ID := ev1.(session.TurnStartedOutput).TurnID

	// Provider emits audio to enter turnStateGenerating
	pSess.EmitAudio(turn1ID, []byte("t1-audio"))
	_ = readEventWithTimeout(t, link.Events())

	// Interrupt turn 1 with turn 2
	_ = link.Send(ctx, session.ClientTextInput{Text: "turn 2"})
	_ = readEventWithTimeout(t, link.Events()) // TurnInterruptedOutput
	evStart2 := readEventWithTimeout(t, link.Events())
	turn2ID := evStart2.(session.TurnStartedOutput).TurnID

	// Provider sends late interrupted event for superseded turn 1
	pSess.EmitInterrupted(turn1ID)

	// Now provider emits audio for turn 2; it must arrive normally (turn 2 was not interrupted!)
	pSess.EmitAudio(turn2ID, []byte("turn2-data"))
	evAudio := readEventWithTimeout(t, link.Events())
	aud, ok := evAudio.(session.AudioOutput)
	if !ok || aud.TurnID != turn2ID || string(aud.Data) != "turn2-data" {
		t.Fatalf("expected turn 2 audio, got %#v", evAudio)
	}
}

func TestStaleProviderTurnStartedIgnoredAndTransitionToGenerating(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Client sends text: turn_1 starts (state: turnStateWaitingForProvider)
	_ = link.Send(ctx, session.ClientTextInput{Text: "hello"})
	ev1 := readEventWithTimeout(t, link.Events())
	turn1ID := ev1.(session.TurnStartedOutput).TurnID

	// Stale provider turn started event with different turn ID should be ignored
	pSess.EmitTurnStarted(session.TurnID("stale_turn_0"))

	// Provider emits matching turn started event and audio chunk: transitions to turnStateGenerating
	pSess.EmitTurnStarted(turn1ID)
	pSess.EmitAudio(turn1ID, []byte("generating-audio"))
	_ = readEventWithTimeout(t, link.Events()) // read generating-audio

	// User barge-in: client sends audio while in generating state -> should trigger turn interruption!
	_ = link.Send(ctx, session.ClientAudioInput{Data: []byte("barge-in-pcm")})

	evInt := readEventWithTimeout(t, link.Events())
	interrupted, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || interrupted.TurnID != turn1ID {
		t.Fatalf("expected TurnInterruptedOutput for turn 1 upon barge-in, got %#v", evInt)
	}

	evStart2 := readEventWithTimeout(t, link.Events())
	if _, ok := evStart2.(session.TurnStartedOutput); !ok {
		t.Fatalf("expected TurnStartedOutput for turn 2, got %#v", evStart2)
	}
}

func TestStaleProviderTextIgnoredAndStateTransition(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	pSess := provider.Sessions()[0]

	// Start turn 1
	_ = link.Send(ctx, session.ClientTextInput{Text: "t1"})
	ev1 := readEventWithTimeout(t, link.Events())
	turn1ID := ev1.(session.TurnStartedOutput).TurnID

	// Provider emits text for turn 1 to transition to turnStateGenerating
	pSess.EmitText(turn1ID, "t1-text")
	_ = readEventWithTimeout(t, link.Events())

	// Interrupt with turn 2
	_ = link.Send(ctx, session.ClientTextInput{Text: "t2"})
	_ = readEventWithTimeout(t, link.Events()) // Interrupted
	evStart2 := readEventWithTimeout(t, link.Events())
	turn2ID := evStart2.(session.TurnStartedOutput).TurnID

	// Provider emits stale text from turn 1
	pSess.EmitText(turn1ID, "stale-text")

	// Provider emits fresh text for turn 2 (transitions turnStateWaitingForProvider -> turnStateGenerating)
	pSess.EmitText(turn2ID, "fresh-text")

	evText := readEventWithTimeout(t, link.Events())
	txt, ok := evText.(session.TextOutput)
	if !ok || txt.TurnID != turn2ID || txt.Text != "fresh-text" {
		t.Fatalf("expected fresh-text for turn 2, got %#v", evText)
	}

	// Because ProviderTextEvent transitioned state to turnStateGenerating, user audio must barge-in/interrupt turn 2!
	_ = link.Send(ctx, session.ClientAudioInput{Data: []byte("barge-in")})

	evInt := readEventWithTimeout(t, link.Events())
	interrupted, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || interrupted.TurnID != turn2ID {
		t.Fatalf("expected TurnInterruptedOutput for turn 2, got %#v", evInt)
	}
}

func TestSupersededLinkDetachDoesNotAffectActiveLink(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link1, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach link1 error: %v", err)
	}

	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach link2 error: %v", err)
	}

	// Detach superseded link1
	if err := link1.Detach(); err != nil {
		t.Fatalf("unexpected detach link1 error: %v", err)
	}

	// Active link2 must continue working normally
	if err := link2.Send(ctx, session.ClientTextInput{Text: "hello link2"}); err != nil {
		t.Fatalf("unexpected send error on link2: %v", err)
	}

	evStart := readEventWithTimeout(t, link2.Events())
	start, ok := evStart.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput on link2, got %#v", evStart)
	}

	pSess := provider.Sessions()[0]
	pSess.EmitAudio(start.TurnID, []byte("resp-link2"))

	evAudio := readEventWithTimeout(t, link2.Events())
	aud, ok := evAudio.(session.AudioOutput)
	if !ok || aud.TurnID != start.TurnID {
		t.Fatalf("expected AudioOutput on active link2, got %#v", evAudio)
	}
}

func TestMediaQueueFastPathDecrementsMediaCount(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Set small capacity of 5
	session.SetMediaQueueCapacityForTesting(link, 5)

	pSess := provider.Sessions()[0]
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)

	evStart := readEventWithTimeout(t, link.Events())
	if _, ok := evStart.(session.TurnStartedOutput); !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", evStart)
	}

	// Stream 20 chunks sequentially, reading each immediately so fast path is taken
	for i := 0; i < 20; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("chunk-%d", i)))
		ev := readEventWithTimeout(t, link.Events())
		aud, ok := ev.(session.AudioOutput)
		if !ok || string(aud.Data) != fmt.Sprintf("chunk-%d", i) {
			t.Fatalf("expected chunk-%d, got %#v", i, ev)
		}
	}

	// Because receiver read each chunk immediately, zero chunks were dropped
	dropped := session.GetDroppedMediaCountForTesting(link)
	if dropped != 0 {
		t.Fatalf("expected 0 dropped chunks, got %d", dropped)
	}
}

func TestMediaQueueDropOldestMaintainsCapacity(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	session.SetMediaQueueCapacityForTesting(link, 3)

	pSess := provider.Sessions()[0]
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)
	_ = readEventWithTimeout(t, link.Events()) // TurnStartedOutput

	// Emit 10 chunks without reading
	for i := 0; i < 10; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("item-%d", i)))
	}

	session.WaitForDroppedMediaCountForTesting(link, 7)

	// Read the 3 items (must be freshest: 7, 8, 9)
	for i := 7; i < 10; i++ {
		ev := readEventWithTimeout(t, link.Events())
		aud, ok := ev.(session.AudioOutput)
		if !ok || string(aud.Data) != fmt.Sprintf("item-%d", i) {
			t.Fatalf("expected item-%d, got %#v", i, ev)
		}
	}

	// Wait for queue to be fully drained
	session.WaitForMediaCountForTesting(link, 0)

	// Now emit 3 more chunks; none should be dropped because queue was drained
	for i := 10; i < 13; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("item-%d", i)))
	}

	session.WaitForMediaCountForTesting(link, 3)

	if dropped := session.GetDroppedMediaCountForTesting(link); dropped != 7 {
		t.Fatalf("expected dropped count to remain 7, got %d", dropped)
	}

	for i := 10; i < 13; i++ {
		ev := readEventWithTimeout(t, link.Events())
		aud, ok := ev.(session.AudioOutput)
		if !ok || string(aud.Data) != fmt.Sprintf("item-%d", i) {
			t.Fatalf("expected item-%d, got %#v", i, ev)
		}
	}
}

func TestInvalidatePlaybackPurgeRestoresCapacity(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	session.SetMediaQueueCapacityForTesting(link, 3)

	// Direct enqueue 3 audio chunks with epoch 1
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_1", Data: []byte("t1-1")}, 1)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_1", Data: []byte("t1-2")}, 1)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_1", Data: []byte("t1-3")}, 1)

	// Invalidate epoch 1
	session.InvalidatePlaybackForTesting(link, 1)

	// Dropped count must be 3 from the purge of epoch 1
	if dropped := session.GetDroppedMediaCountForTesting(link); dropped != 3 {
		t.Fatalf("expected dropped count 3 (from purged turn 1), got %d", dropped)
	}

	// Now enqueue 3 audio chunks for epoch 2 without reading
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_2", Data: []byte("t2-1")}, 2)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_2", Data: []byte("t2-2")}, 2)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_2", Data: []byte("t2-3")}, 2)

	// Since queue was purged, none of epoch 2 chunks should be dropped!
	if dropped := session.GetDroppedMediaCountForTesting(link); dropped != 3 {
		t.Fatalf("expected dropped count to remain 3, got %d", dropped)
	}

	for _, expected := range []string{"t2-1", "t2-2", "t2-3"} {
		ev := readEventWithTimeout(t, link.Events())
		aud, ok := ev.(session.AudioOutput)
		if !ok || string(aud.Data) != expected {
			t.Fatalf("expected %q, got %#v", expected, ev)
		}
	}
}

func TestSetMaxMediaQueueInvalidCapacityIgnored(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Valid capacity: 4
	session.SetMediaQueueCapacityForTesting(link, 4)

	// Invalid capacity calls: <= 0 should be ignored and keep 4
	session.SetMediaQueueCapacityForTesting(link, 0)
	session.SetMediaQueueCapacityForTesting(link, -5)

	pSess := provider.Sessions()[0]
	turnID := session.TurnID("turn_1")
	pSess.EmitTurnStarted(turnID)
	_ = readEventWithTimeout(t, link.Events())

	// Emit 6 chunks
	for i := 0; i < 6; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("c-%d", i)))
	}

	session.WaitForDroppedMediaCountForTesting(link, 2)
	dropped := session.GetDroppedMediaCountForTesting(link)
	if dropped != 2 {
		t.Fatalf("expected 2 dropped chunks under capacity 4, got %d", dropped)
	}
}

func TestEnqueueMediaWithInvalidatedEpochBoundary(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Invalidate playback up to epoch 5
	session.InvalidatePlaybackForTesting(link, 5)

	// Direct enqueue with epoch == 5 (must be dropped immediately by boundary check epoch <= invalidatedEpoch)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_old", Data: []byte("old-5")}, 5)

	// Direct enqueue with epoch == 4 (must also be dropped)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_old", Data: []byte("old-4")}, 4)

	if dropped := session.GetDroppedMediaCountForTesting(link); dropped != 2 {
		t.Fatalf("expected 2 dropped chunks for epochs <= 5, got %d", dropped)
	}

	// Direct enqueue with epoch == 6 (must NOT be dropped)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "turn_new", Data: []byte("new-6")}, 6)

	ev := readEventWithTimeout(t, link.Events())
	aud, ok := ev.(session.AudioOutput)
	if !ok || string(aud.Data) != "new-6" {
		t.Fatalf("expected new-6 audio, got %#v", ev)
	}
}

func TestWaitForDroppedMediaCountOnPurge(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	// Enqueue 2 media items with epoch 1
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "t1", Data: []byte("d1")}, 1)
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: "t1", Data: []byte("d2")}, 1)

	doneCh := make(chan int64, 1)
	go func() {
		cnt := session.WaitForDroppedMediaCountForTesting(link, 2)
		doneCh <- cnt
	}()

	// Invalidate epoch 1 which purges the 2 items and broadcasts queueCond
	session.InvalidatePlaybackForTesting(link, 1)

	select {
	case cnt := <-doneCh:
		if cnt != 2 {
			t.Fatalf("expected 2 dropped chunks, got %d", cnt)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for WaitForDroppedMediaCount on purge")
	}
}

func TestMediaQueueDropWhileDispatchWaitingMaintainsCapacity(t *testing.T) {
	ctx := context.Background()
	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("unexpected attach error: %v", err)
	}

	capacity := 3
	session.SetMediaQueueCapacityForTesting(link, capacity)

	turnID := session.TurnID("turn_1")

	inSlowPath := make(chan struct{})
	var dropOnce sync.Once
	session.SetOnDispatchWaitingForTesting(link, func() {
		dropOnce.Do(func() {
			// At this exact moment, dispatchLoop has copied chunk-0 into nextItem
			// and released queueMu. Enqueuing chunks 1, 2, and 3 fills the queue to capacity (3)
			// and chunk-3 drops chunk-0 from link.queue.
			session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: turnID, Data: []byte("chunk-1")}, 1)
			session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: turnID, Data: []byte("chunk-2")}, 1)
			session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: turnID, Data: []byte("chunk-3")}, 1)
			close(inSlowPath)
		})
	})

	// Emit chunk-0 directly to link
	session.EnqueueMediaDirectForTesting(link, session.AudioOutput{TurnID: turnID, Data: []byte("chunk-0")}, 1)

	// Wait until dispatchLoop is waiting to deliver chunk-0
	<-inSlowPath

	// Read events until all chunks are consumed.
	var receivedAudio []string
	for i := 0; i < capacity; i++ {
		t.Logf("reading chunk %d", i)
		ev := readEventWithTimeout(t, link.Events())
		if aud, ok := ev.(session.AudioOutput); ok {
			t.Logf("received chunk: %s", string(aud.Data))
			receivedAudio = append(receivedAudio, string(aud.Data))
		} else {
			t.Fatalf("unexpected event: %#v", ev)
		}
	}

	// Verify that no extra chunks leaked beyond capacity (capacity is 3)
	select {
	case extra := <-link.Events():
		if aud, ok := extra.(session.AudioOutput); ok {
			t.Fatalf("audio chunks delivered exceeded max capacity (%d): got extra chunk %s", capacity, string(aud.Data))
		}
	case <-time.After(50 * time.Millisecond):
		// Expected: no extra chunks delivered
	}

	if len(receivedAudio) != capacity {
		t.Fatalf("expected exactly %d audio chunks delivered, got %d: %v", capacity, len(receivedAudio), receivedAudio)
	}

	// The newest chunk (chunk-3) must be the last chunk received
	if last := receivedAudio[len(receivedAudio)-1]; last != "chunk-3" {
		t.Fatalf("expected freshest chunk-3, got %s", last)
	}
}
