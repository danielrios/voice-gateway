package session

import (
	"context"
	"fmt"
)

type turnState int

const (
	turnStateIdle turnState = iota
	turnStateWaitingForProvider
	turnStateGenerating
)

type attachResult struct {
	link ClientLink
	err  error
}

type attachCmd struct {
	reply chan attachResult
}

type detachCmd struct {
	link *clientLink
}

type endCmd struct {
	reply chan error
}

type inputCmd struct {
	ctx   context.Context
	link  *clientLink
	input SessionInput
	reply chan error
}

type audioCmd struct {
	link  *clientLink
	input ClientAudioInput
}

type handle struct {
	id       SessionID
	provider ProviderSession
	runtime  RuntimeSession

	ctx       context.Context
	cancel    context.CancelFunc
	cmdChan   chan any
	audioChan chan audioCmd
	done      chan struct{}
}

func newHandle(id SessionID, provider ProviderSession, runtime RuntimeSession) *handle {
	ctx, cancel := context.WithCancel(context.Background())
	h := &handle{
		id:        id,
		provider:  provider,
		runtime:   runtime,
		ctx:       ctx,
		cancel:    cancel,
		cmdChan:   make(chan any, 64),
		audioChan: make(chan audioCmd, 64),
		done:      make(chan struct{}),
	}
	go h.run()
	return h
}

func (h *handle) ID() SessionID {
	return h.id
}

func (h *handle) sendAudio(ctx context.Context, l *clientLink, in ClientAudioInput) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return ErrSessionEnded
	default:
	}

	cmd := audioCmd{link: l, input: in}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return ErrSessionEnded
	case h.audioChan <- cmd:
		return nil
	default:
		// Drop oldest unread media chunk under backpressure.
		select {
		case <-h.audioChan:
		default:
		}
		select {
		case h.audioChan <- cmd:
		default:
		}
		return nil
	}
}

func (h *handle) run() {
	var (
		activeLink                *clientLink
		turnEpoch                 uint64
		currentTurnID             TurnID
		currentTurnState          turnState = turnStateIdle
		currentTurnStartedByAudio bool
		invalidatedTurnIDs        = make(map[TurnID]struct{})
	)

	var providerEvents <-chan ProviderEvent
	if h.provider != nil {
		providerEvents = h.provider.Events()
	}

	cleanup := func() {
		if activeLink != nil {
			activeLink.terminate()
			activeLink = nil
		}
		if h.provider != nil {
			_ = h.provider.Close()
		}
		if h.runtime != nil {
			_ = h.runtime.Close()
		}
		h.cancel()
	}

	drainCommands := func() {
		for {
			select {
			case msg := <-h.cmdChan:
				switch cmd := msg.(type) {
				case attachCmd:
					cmd.reply <- attachResult{err: ErrSessionEnded}
				case inputCmd:
					cmd.reply <- ErrSessionEnded
				case endCmd:
					cmd.reply <- nil
				}
			default:
				return
			}
		}
	}

	invalidateCurrentTurn := func() {
		currentTurnStartedByAudio = false
		if currentTurnID == "" {
			return
		}
		interruptedID := currentTurnID
		interruptedEpoch := turnEpoch
		invalidatedTurnIDs[interruptedID] = struct{}{}
		currentTurnID = ""
		if activeLink != nil {
			activeLink.invalidatePlayback(interruptedEpoch)
			activeLink.enqueueControl(TurnInterruptedOutput{TurnID: interruptedID})
		}
		if h.provider != nil {
			_ = h.provider.Interrupt(context.Background())
		}
	}

	startTurn := func(preferredID TurnID, nextState turnState) TurnID {
		turnEpoch++
		if preferredID != "" {
			currentTurnID = preferredID
		} else {
			currentTurnID = TurnID(fmt.Sprintf("turn_%d", turnEpoch))
		}
		currentTurnState = nextState
		if activeLink != nil {
			activeLink.enqueueControl(TurnStartedOutput{TurnID: currentTurnID})
		}
		return currentTurnID
	}

	stop := func() {
		cleanup()
		close(h.done)
		drainCommands()
	}

	handleProviderEvent := func(ev ProviderEvent) {
		var evTurnID TurnID
		switch e := ev.(type) {
		case ProviderTurnStartedEvent:
			evTurnID = e.TurnID
		case ProviderAudioEvent:
			evTurnID = e.TurnID
		case ProviderTextEvent:
			evTurnID = e.TurnID
		case ProviderTurnCompletedEvent:
			evTurnID = e.TurnID
		case ProviderInterruptedEvent:
			evTurnID = e.TurnID
		}
		if evTurnID != "" {
			if _, invalidated := invalidatedTurnIDs[evTurnID]; invalidated {
				return
			}
		}

		switch e := ev.(type) {
		case ProviderTurnStartedEvent:
			if currentTurnState == turnStateWaitingForProvider && currentTurnStartedByAudio {
				if e.TurnID != "" {
					currentTurnID = e.TurnID
				}
				currentTurnState = turnStateGenerating
				currentTurnStartedByAudio = false
				return
			}
			if e.TurnID != "" && currentTurnID != "" && e.TurnID != currentTurnID {
				return
			}
			if currentTurnState == turnStateIdle {
				startTurn(e.TurnID, turnStateGenerating)
			} else {
				if e.TurnID != "" && currentTurnID == "" {
					currentTurnID = e.TurnID
				}
				currentTurnState = turnStateGenerating
			}

		case ProviderAudioEvent:
			if currentTurnState == turnStateWaitingForProvider && currentTurnStartedByAudio {
				if e.TurnID != "" {
					currentTurnID = e.TurnID
				}
				currentTurnState = turnStateGenerating
				currentTurnStartedByAudio = false
			} else if e.TurnID != "" && e.TurnID != currentTurnID {
				return
			}
			if currentTurnState == turnStateIdle {
				startTurn(e.TurnID, turnStateGenerating)
			} else if currentTurnState == turnStateWaitingForProvider {
				currentTurnState = turnStateGenerating
			}
			if activeLink != nil {
				activeLink.enqueueMedia(AudioOutput{TurnID: currentTurnID, Data: e.Data}, turnEpoch)
			}

		case ProviderTextEvent:
			if currentTurnState == turnStateWaitingForProvider && currentTurnStartedByAudio {
				if e.TurnID != "" {
					currentTurnID = e.TurnID
				}
				currentTurnState = turnStateGenerating
				currentTurnStartedByAudio = false
			} else if e.TurnID != "" && e.TurnID != currentTurnID {
				return
			}
			if currentTurnState == turnStateIdle {
				startTurn(e.TurnID, turnStateGenerating)
			} else if currentTurnState == turnStateWaitingForProvider {
				currentTurnState = turnStateGenerating
			}
			if activeLink != nil {
				activeLink.enqueueControl(TextOutput{TurnID: currentTurnID, Text: e.Text})
			}

		case ProviderTurnCompletedEvent:
			if e.TurnID != "" && currentTurnID != "" && e.TurnID != currentTurnID {
				return
			}
			if currentTurnState == turnStateIdle {
				return
			}
			completedID := currentTurnID
			currentTurnState = turnStateIdle
			currentTurnID = ""
			currentTurnStartedByAudio = false
			if activeLink != nil {
				activeLink.enqueueControl(TurnCompletedOutput{TurnID: completedID})
			}

		case ProviderInterruptedEvent:
			if e.TurnID != "" && e.TurnID != currentTurnID {
				return
			}
			if currentTurnState != turnStateIdle {
				invalidateCurrentTurn()
				currentTurnState = turnStateIdle
			}
		}
	}

	handleAudioCmd := func(cmd audioCmd) {
		if activeLink != cmd.link {
			return
		}
		if currentTurnState == turnStateGenerating {
			invalidateCurrentTurn()
			startTurn("", turnStateWaitingForProvider)
			currentTurnStartedByAudio = true
		} else if currentTurnState == turnStateIdle {
			startTurn("", turnStateWaitingForProvider)
			currentTurnStartedByAudio = true
		}
		if h.provider != nil {
			_ = h.provider.SendAudio(h.ctx, cmd.input.Data)
		}
	}

	handleInputCmd := func(cmd inputCmd) {
		if activeLink != cmd.link {
			cmd.reply <- ErrLinkSuperseded
			return
		}

		switch in := cmd.input.(type) {
		case ClientAudioInput:
			if currentTurnState == turnStateGenerating {
				invalidateCurrentTurn()
				startTurn("", turnStateWaitingForProvider)
				currentTurnStartedByAudio = true
			} else if currentTurnState == turnStateIdle {
				startTurn("", turnStateWaitingForProvider)
				currentTurnStartedByAudio = true
			}
			if h.provider != nil {
				_ = h.provider.SendAudio(h.ctx, in.Data)
			}
			cmd.reply <- nil

		case ClientTextInput:
			if currentTurnState == turnStateGenerating {
				invalidateCurrentTurn()
			}
			currentTurnStartedByAudio = false
			startTurn("", turnStateWaitingForProvider)
			if h.provider != nil {
				if err := h.provider.SendText(cmd.ctx, in.Text); err != nil {
					cmd.reply <- err
					return
				}
			}
			cmd.reply <- nil

		case ClientInterruptedInput:
			if currentTurnState != turnStateIdle {
				invalidateCurrentTurn()
				currentTurnState = turnStateIdle
			}
			cmd.reply <- nil

		default:
			cmd.reply <- nil
		}
	}

	processCmd := func(msg any) bool {
		switch cmd := msg.(type) {
		case attachCmd:
			if activeLink != nil {
				activeLink.supersede()
			}
			link := newClientLink(
				func(l *clientLink) {
					select {
					case h.cmdChan <- detachCmd{link: l}:
					case <-h.done:
					}
				},
				func(ctx context.Context, l *clientLink, in SessionInput) error {
					reply := make(chan error, 1)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-h.done:
						return ErrSessionEnded
					case h.cmdChan <- inputCmd{ctx: ctx, link: l, input: in, reply: reply}:
					}

					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-h.done:
						return ErrSessionEnded
					case err := <-reply:
						return err
					}
				},
			)
			activeLink = link
			cmd.reply <- attachResult{link: link}

		case detachCmd:
			if activeLink == cmd.link {
				activeLink = nil
			}

		case inputCmd:
			handleInputCmd(cmd)

		case endCmd:
			stop()
			cmd.reply <- nil
			return true
		}
		return false
	}

	for {
		select {
		case <-h.ctx.Done():
			stop()
			return

		case msg := <-h.cmdChan:
			if processCmd(msg) {
				return
			}

		default:
			select {
			case <-h.ctx.Done():
				stop()
				return

			case msg := <-h.cmdChan:
				if processCmd(msg) {
					return
				}

			case cmd := <-h.audioChan:
				handleAudioCmd(cmd)

			case ev, ok := <-providerEvents:
				if !ok {
					providerEvents = nil
					continue
				}
				handleProviderEvent(ev)
				for providerEvents != nil {
					select {
					case nextEv, nextOk := <-providerEvents:
						if !nextOk {
							providerEvents = nil
							break
						}
						handleProviderEvent(nextEv)
						continue
					default:
					}
					break
				}
			}
		}
	}
}

func (h *handle) Attach(ctx context.Context) (ClientLink, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	reply := make(chan attachResult, 1)
	cmd := attachCmd{reply: reply}

	select {
	case <-h.done:
		return nil, ErrSessionEnded
	case <-ctx.Done():
		return nil, ctx.Err()
	case h.cmdChan <- cmd:
	}

	select {
	case <-h.done:
		return nil, ErrSessionEnded
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-reply:
		return res.link, res.err
	}
}

func (h *handle) End(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	reply := make(chan error, 1)
	cmd := endCmd{reply: reply}

	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case h.cmdChan <- cmd:
	}

	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case err := <-reply:
		return err
	}
}
