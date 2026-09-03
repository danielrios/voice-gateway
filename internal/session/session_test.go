package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/danielrios/voice-gateway/internal/session"
)

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

	// Verify events channel is closed upon detach
	for range link.Events() {
		// drain any remaining events
	}

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

	// Verify link1 is now terminal / superseded:
	// Events channel should be closed.
	select {
	case _, ok := <-link1.Events():
		if ok {
			t.Fatal("expected link1 events channel to be closed, but received event")
		}
	default:
		// channel might take a moment if not closed immediately; but our implementation closes synchronously on attach
		// Let's check with channel read
		_, ok := <-link1.Events()
		if ok {
			t.Fatal("expected link1 events channel to be closed")
		}
	}

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
	select {
	case _, ok := <-link.Events():
		if ok {
			t.Fatal("expected active link events channel to be closed after End, but received event")
		}
	default:
		_, ok := <-link.Events()
		if ok {
			t.Fatal("expected active link events channel to be closed after End")
		}
	}

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
				// ErrSessionEnded is valid if End raced ahead
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
