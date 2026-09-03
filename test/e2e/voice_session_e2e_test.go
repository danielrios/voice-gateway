package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/danielrios/voice-gateway/internal/session"
)

// simulatedVoiceClient represents an end-user client device (e.g. mobile app or smart speaker)
// that connects to the Voice Gateway via ClientLink, sends microphone audio/text, and plays speaker audio.
type simulatedVoiceClient struct {
	t        *testing.T
	link     session.ClientLink
	received []session.SessionOutput
	audioOut [][]byte
	texts    []string
	turns    []session.TurnID
	mu       sync.Mutex
	doneChan chan struct{}
}

func newSimulatedVoiceClient(t *testing.T, link session.ClientLink) *simulatedVoiceClient {
	c := &simulatedVoiceClient{
		t:        t,
		link:     link,
		doneChan: make(chan struct{}),
	}
	go c.listen()
	return c
}

func (c *simulatedVoiceClient) listen() {
	defer close(c.doneChan)
	for out := range c.link.Events() {
		c.mu.Lock()
		c.received = append(c.received, out)
		switch o := out.(type) {
		case session.AudioOutput:
			c.audioOut = append(c.audioOut, o.Data)
		case session.TextOutput:
			c.texts = append(c.texts, o.Text)
		case session.TurnStartedOutput:
			c.turns = append(c.turns, o.TurnID)
		}
		c.mu.Unlock()
	}
}

func (c *simulatedVoiceClient) ReadNextEvent(timeout time.Duration) (session.SessionOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		c.mu.Lock()
		if len(c.received) > 0 {
			ev := c.received[0]
			c.received = c.received[1:]
			c.mu.Unlock()
			return ev, nil
		}
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for next event: %w", ctx.Err())
		default:
			// Check if link events channel closed
			select {
			case <-c.doneChan:
				c.mu.Lock()
				if len(c.received) > 0 {
					ev := c.received[0]
					c.received = c.received[1:]
					c.mu.Unlock()
					return ev, nil
				}
				c.mu.Unlock()
				return nil, fmt.Errorf("client link closed")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

// User Journey 1: Happy Path Spoken Conversation Turn
func TestE2E_SpokenTurnLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	defer func() { _ = link.Detach() }()

	client := newSimulatedVoiceClient(t, link)

	// User speaks PCM audio chunks into microphone
	userAudioChunks := [][]byte{
		[]byte("pcm-chunk-1-user-hello"),
		[]byte("pcm-chunk-2-what-time-is-it"),
	}
	for _, chunk := range userAudioChunks {
		if err := link.Send(ctx, session.ClientAudioInput{Data: chunk}); err != nil {
			t.Fatalf("Send ClientAudioInput failed: %v", err)
		}
	}

	// Verify gateway forwarded user audio to the provider
	pSessions := provider.Sessions()
	if len(pSessions) != 1 {
		t.Fatalf("expected 1 provider session, got %d", len(pSessions))
	}
	pSess := pSessions[0]
	receivedAudio := pSess.WaitForAudioCount(2)
	if len(receivedAudio) < 2 {
		t.Fatalf("provider did not receive user audio chunks")
	}

	// Client receives TurnStartedOutput
	ev1, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 1: %v", err)
	}
	startOut, ok := ev1.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", ev1)
	}
	turnID := startOut.TurnID

	// Provider responds: TurnStarted, Text chunks, Audio chunks, TurnCompleted
	pSess.EmitTurnStarted(turnID)
	pSess.EmitText(turnID, "The time is ")
	pSess.EmitText(turnID, "3:00 PM.")
	pSess.EmitAudio(turnID, []byte("pcm-audio-chunk-1"))
	pSess.EmitAudio(turnID, []byte("pcm-audio-chunk-2"))
	pSess.EmitTurnCompleted(turnID)

	// Client receives TextOutput "The time is "
	ev2, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 2: %v", err)
	}
	text1, ok := ev2.(session.TextOutput)
	if !ok || text1.Text != "The time is " {
		t.Fatalf("expected TextOutput 'The time is ', got %#v", ev2)
	}

	// Client receives TextOutput "3:00 PM."
	ev3, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 3: %v", err)
	}
	text2, ok := ev3.(session.TextOutput)
	if !ok || text2.Text != "3:00 PM." {
		t.Fatalf("expected TextOutput '3:00 PM.', got %#v", ev3)
	}

	// Client receives AudioOutput chunk 1
	ev4, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 4: %v", err)
	}
	audio1, ok := ev4.(session.AudioOutput)
	if !ok || !bytes.Equal(audio1.Data, []byte("pcm-audio-chunk-1")) {
		t.Fatalf("expected AudioOutput chunk 1, got %#v", ev4)
	}

	// Client receives AudioOutput chunk 2
	ev5, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 5: %v", err)
	}
	audio2, ok := ev5.(session.AudioOutput)
	if !ok || !bytes.Equal(audio2.Data, []byte("pcm-audio-chunk-2")) {
		t.Fatalf("expected AudioOutput chunk 2, got %#v", ev5)
	}

	// Client receives TurnCompletedOutput
	ev6, err := client.ReadNextEvent(2 * time.Second)
	if err != nil {
		t.Fatalf("failed reading event 6: %v", err)
	}
	compOut, ok := ev6.(session.TurnCompletedOutput)
	if !ok {
		t.Fatalf("expected TurnCompletedOutput, got %#v", ev6)
	}
	if compOut.TurnID != startOut.TurnID {
		t.Fatalf("expected completed TurnID %s, got %s", startOut.TurnID, compOut.TurnID)
	}
}

// User Journey 2: User Barge-In Interruption during Realtime Streaming Playback
func TestE2E_UserBargeInInterruptionDuringPlayback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	defer func() { _ = link.Detach() }()

	pSess := provider.Sessions()[0]

	// User asks a question
	if err := link.Send(ctx, session.ClientTextInput{Text: "Tell me a story"}); err != nil {
		t.Fatalf("Send text failed: %v", err)
	}

	// Read initial TurnStartedOutput
	evStart1 := <-link.Events()
	start1, ok := evStart1.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput, got %#v", evStart1)
	}
	turn1ID := start1.TurnID

	// Provider begins streaming audio response for Turn 1
	pSess.EmitTurnStarted(turn1ID)
	pSess.EmitAudio(turn1ID, []byte("story-part-1"))
	pSess.EmitAudio(turn1ID, []byte("story-part-2-queued"))
	pSess.EmitAudio(turn1ID, []byte("story-part-3-queued"))

	// Client speaker plays only part-1
	evPart1 := <-link.Events()
	audio1, ok := evPart1.(session.AudioOutput)
	if !ok || !bytes.Equal(audio1.Data, []byte("story-part-1")) {
		t.Fatalf("expected story-part-1, got %#v", evPart1)
	}

	// While part-2 and part-3 are queued at the gateway link, user barges in!
	// User speaks: "Wait, stop that, tell me a joke instead!"
	if err := link.Send(ctx, session.ClientAudioInput{Data: []byte("user-barge-in-speech")}); err != nil {
		t.Fatalf("Send barge-in audio failed: %v", err)
	}

	// Gateway must immediately:
	// 1. Invalidate queued playback (story-part-2 and story-part-3 must NEVER be delivered)
	// 2. Deliver TurnInterruptedOutput for turn 1
	// 3. Signal ProviderSession.Interrupt()
	// 4. Deliver TurnStartedOutput for turn 2

	evInt := <-link.Events()
	intOut, ok := evInt.(session.TurnInterruptedOutput)
	if !ok {
		t.Fatalf("expected TurnInterruptedOutput, got %#v", evInt)
	}
	if intOut.TurnID != turn1ID {
		t.Fatalf("expected TurnInterruptedOutput for %s, got %s", turn1ID, intOut.TurnID)
	}

	// Provider must have been interrupted
	if pSess.InterruptCount() != 1 {
		t.Fatalf("expected provider interrupt count 1, got %d", pSess.InterruptCount())
	}

	evStart2 := <-link.Events()
	start2, ok := evStart2.(session.TurnStartedOutput)
	if !ok {
		t.Fatalf("expected TurnStartedOutput for turn 2, got %#v", evStart2)
	}
	if start2.TurnID == turn1ID {
		t.Fatalf("turn 2 should have a different turn ID from turn 1")
	}
	turn2ID := start2.TurnID

	// Stale provider generation from Turn 1 (audio and completion) arrives late
	pSess.EmitAudio(turn1ID, []byte("stale-story-part-4"))
	pSess.EmitTurnCompleted(turn1ID)

	// Provider now produces output for Turn 2
	pSess.EmitAudio(turn2ID, []byte("joke-audio-punchline"))
	pSess.EmitTurnCompleted(turn2ID)

	// Next event MUST be Turn 2 audio; stale Turn 1 audio/completion MUST NOT appear!
	evTurn2Audio := <-link.Events()
	audioTurn2, ok := evTurn2Audio.(session.AudioOutput)
	if !ok || !bytes.Equal(audioTurn2.Data, []byte("joke-audio-punchline")) {
		t.Fatalf("expected joke-audio-punchline, got %#v", evTurn2Audio)
	}
	if audioTurn2.TurnID != turn2ID {
		t.Fatalf("expected AudioOutput for turn 2 (%s), got %s", turn2ID, audioTurn2.TurnID)
	}

	evTurn2Comp := <-link.Events()
	comp2, ok := evTurn2Comp.(session.TurnCompletedOutput)
	if !ok || comp2.TurnID != turn2ID {
		t.Fatalf("expected TurnCompletedOutput for turn 2, got %#v", evTurn2Comp)
	}
}

// User Journey 3: Explicit User Interruption (Push-to-Talk / Mute Button)
func TestE2E_ExplicitUserInterruptedInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	defer func() { _ = link.Detach() }()

	pSess := provider.Sessions()[0]

	// User speaks
	_ = link.Send(ctx, session.ClientTextInput{Text: "Start playing music"})
	evStart := <-link.Events()
	turnID := evStart.(session.TurnStartedOutput).TurnID

	pSess.EmitTurnStarted(turnID)
	pSess.EmitAudio(turnID, []byte("music-chunk-1"))
	pSess.EmitAudio(turnID, []byte("music-chunk-2"))

	_ = <-link.Events() // chunk 1

	// User hits "Mute" / "Stop" button on client
	if err := link.Send(ctx, session.ClientInterruptedInput{}); err != nil {
		t.Fatalf("Send ClientInterruptedInput failed: %v", err)
	}

	// Must receive TurnInterruptedOutput
	evInt := <-link.Events()
	intOut, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || intOut.TurnID != turnID {
		t.Fatalf("expected TurnInterruptedOutput for %s, got %#v", turnID, evInt)
	}

	// Provider must have been interrupted
	if pSess.InterruptCount() != 1 {
		t.Fatalf("expected provider interrupted")
	}

	// Late provider audio for turn 1 is discarded
	pSess.EmitAudio(turnID, []byte("late-music-chunk-3"))
	pSess.EmitTurnCompleted(turnID)

	// User speaks again -> new turn starts cleanly
	_ = link.Send(ctx, session.ClientTextInput{Text: "New query"})
	evStart2 := <-link.Events()
	start2, ok := evStart2.(session.TurnStartedOutput)
	if !ok || start2.TurnID == turnID {
		t.Fatalf("expected new turn start, got %#v", evStart2)
	}
}

// User Journey 4: Provider-Side Voice Activity Detection (Barge-In)
func TestE2E_ProviderVADBargeInDetection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	defer func() { _ = link.Detach() }()

	pSess := provider.Sessions()[0]

	// Start generation
	turnID := session.TurnID("turn-vad-1")
	pSess.EmitTurnStarted(turnID)

	evStart := <-link.Events()
	if evStart.(session.TurnStartedOutput).TurnID != turnID {
		t.Fatalf("expected turnID %s", turnID)
	}

	pSess.EmitAudio(turnID, []byte("speech-1"))
	_ = <-link.Events() // speech-1

	pSess.EmitAudio(turnID, []byte("speech-2-will-be-purged"))

	// Provider VAD detects user speaking and emits ProviderInterruptedEvent
	pSess.EmitInterrupted(turnID)

	// Client receives TurnInterruptedOutput; speech-2 is purged!
	evInt := <-link.Events()
	intOut, ok := evInt.(session.TurnInterruptedOutput)
	if !ok || intOut.TurnID != turnID {
		t.Fatalf("expected TurnInterruptedOutput for %s, got %#v", turnID, evInt)
	}

	// Late completion from interrupted turn is discarded
	pSess.EmitTurnCompleted(turnID)

	// Next turn starts cleanly
	newTurnID := session.TurnID("turn-vad-2")
	pSess.EmitTurnStarted(newTurnID)
	evStart2 := <-link.Events()
	if evStart2.(session.TurnStartedOutput).TurnID != newTurnID {
		t.Fatalf("expected new turn %s, got %#v", newTurnID, evStart2)
	}
}

// User Journey 5: Slow Client Speaker & Bounded Media Backpressure (Zero Control Drops)
func TestE2E_SlowClientSpeakerAndBoundedMediaQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	defer func() { _ = link.Detach() }()

	pSess := provider.Sessions()[0]

	turnID := session.TurnID("turn-backpressure")
	pSess.EmitTurnStarted(turnID)
	pSess.EmitText(turnID, "Important control text: Order #1234 confirmed")

	// Emit flood of audio chunks exceeding DefaultMediaQueueCapacity (32 chunks)
	for i := 0; i < 50; i++ {
		pSess.EmitAudio(turnID, []byte(fmt.Sprintf("audio-chunk-%d", i)))
	}

	pSess.EmitTurnCompleted(turnID)

	// Now client consumes from link.
	// Invariant: TurnStartedOutput, TextOutput, and TurnCompletedOutput MUST NEVER be dropped!
	ev1 := <-link.Events()
	startOut, ok := ev1.(session.TurnStartedOutput)
	if !ok || startOut.TurnID != turnID {
		t.Fatalf("expected TurnStartedOutput, got %#v", ev1)
	}

	ev2 := <-link.Events()
	textOut, ok := ev2.(session.TextOutput)
	if !ok || textOut.Text != "Important control text: Order #1234 confirmed" {
		t.Fatalf("expected TextOutput with critical confirmation, got %#v", ev2)
	}

	var receivedAudioChunks []string
	var completedEv session.TurnCompletedOutput
	gotCompleted := false

	// Drain remaining events
	for !gotCompleted {
		select {
		case ev := <-link.Events():
			switch o := ev.(type) {
			case session.AudioOutput:
				receivedAudioChunks = append(receivedAudioChunks, string(o.Data))
			case session.TurnCompletedOutput:
				completedEv = o
				gotCompleted = true
			default:
				t.Fatalf("unexpected event type: %#v", ev)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for TurnCompletedOutput; received %d audio chunks", len(receivedAudioChunks))
		}
	}

	if completedEv.TurnID != turnID {
		t.Fatalf("expected TurnCompletedOutput for %s, got %s", turnID, completedEv.TurnID)
	}

	// Audio chunks should be bounded: older chunks dropped, newer chunks delivered
	if len(receivedAudioChunks) > session.DefaultMediaQueueCapacity {
		t.Fatalf("audio chunks delivered (%d) exceeded max capacity (%d)", len(receivedAudioChunks), session.DefaultMediaQueueCapacity)
	}

	// The newest chunk (audio-chunk-49) must be among the received chunks
	lastChunk := receivedAudioChunks[len(receivedAudioChunks)-1]
	if lastChunk != "audio-chunk-49" {
		t.Fatalf("expected freshest audio chunk 49, got %s", lastChunk)
	}
}

// User Journey 6: Client Network Reconnect & Link Supersession
func TestE2E_ClientNetworkReconnectAndLinkSupersession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = handle.End(ctx) }()

	// Client connection 1 attaches
	link1, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach link1 failed: %v", err)
	}

	// Send audio from link1
	if err := link1.Send(ctx, session.ClientAudioInput{Data: []byte("client1-audio")}); err != nil {
		t.Fatalf("link1 send failed: %v", err)
	}

	// Client connection drops and reconnects as link2
	link2, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach link2 failed: %v", err)
	}
	defer func() { _ = link2.Detach() }()

	// Link1 must be superseded
	err = link1.Send(ctx, session.ClientAudioInput{Data: []byte("late-client1-audio")})
	if err != session.ErrLinkSuperseded {
		t.Fatalf("expected ErrLinkSuperseded on link1, got %v", err)
	}

	// Link2 operates normally
	if err := link2.Send(ctx, session.ClientTextInput{Text: "client2-hello"}); err != nil {
		t.Fatalf("link2 send failed: %v", err)
	}

	pSess := provider.Sessions()[0]
	texts := pSess.ReceivedTexts()
	if len(texts) != 1 || texts[0] != "client2-hello" {
		t.Fatalf("expected provider to receive client2 text, got %#v", texts)
	}

	// Clean detach of link1 is a no-op
	if err := link1.Detach(); err != nil {
		t.Fatalf("link1 Detach should not error: %v", err)
	}
}

// User Journey 7: Explicit Session End and Lifecycle Cleanup
func TestE2E_ExplicitSessionEndLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := session.NewFakeVoiceProvider()
	runtime := session.NewFakeAgentRuntime()
	engine := session.NewEngine(provider, runtime)

	handle, err := engine.Open(ctx, session.OpenRequest{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	link, err := handle.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	// End the session
	if err := handle.End(ctx); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// End is idempotent
	if err := handle.End(ctx); err != nil {
		t.Fatalf("Second End should be idempotent, got %v", err)
	}

	// Attach on ended session returns ErrSessionEnded
	_, err = handle.Attach(ctx)
	if err != session.ErrSessionEnded {
		t.Fatalf("expected ErrSessionEnded on Attach, got %v", err)
	}

	// Link is terminal
	err = link.Send(ctx, session.ClientAudioInput{Data: []byte("post-end-audio")})
	if err != session.ErrLinkTerminal {
		t.Fatalf("expected ErrLinkTerminal on link Send, got %v", err)
	}

	// Provider session is closed
	pSess := provider.Sessions()[0]
	if !pSess.IsClosed() {
		t.Fatalf("expected provider session to be closed")
	}

	// Runtime session is closed
	rSess := runtime.Sessions()[0]
	if !rSess.IsClosed() {
		t.Fatalf("expected runtime session to be closed")
	}
}
