package session

import (
	"context"
	"sync"
)

type linkState int

const (
	linkActive linkState = iota
	linkDetached
	linkSuperseded
	linkTerminated
)

type clientLink struct {
	events   chan SessionOutput
	mu       sync.RWMutex
	state    linkState
	onDetach func(*clientLink)
	onSend   func(context.Context, *clientLink, SessionInput) error
}

func newClientLink(onDetach func(*clientLink), onSend func(context.Context, *clientLink, SessionInput) error) *clientLink {
	return &clientLink{
		events:   make(chan SessionOutput, 64),
		state:    linkActive,
		onDetach: onDetach,
		onSend:   onSend,
	}
}

func (l *clientLink) Events() <-chan SessionOutput {
	return l.events
}

func (l *clientLink) closeWithState(newState linkState) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.state == linkActive {
		l.state = newState
		close(l.events)
		return true
	}
	return false
}

func (l *clientLink) Detach() error {
	if l.closeWithState(linkDetached) && l.onDetach != nil {
		l.onDetach(l)
	}
	return nil
}

func (l *clientLink) supersede() {
	l.closeWithState(linkSuperseded)
}

func (l *clientLink) terminate() {
	l.closeWithState(linkTerminated)
}

func (l *clientLink) Send(ctx context.Context, input SessionInput) error {
	l.mu.RLock()
	st := l.state
	l.mu.RUnlock()

	switch st {
	case linkDetached:
		return ErrLinkDetached
	case linkSuperseded:
		return ErrLinkSuperseded
	case linkTerminated:
		return ErrLinkTerminal
	case linkActive:
		if l.onSend != nil {
			return l.onSend(ctx, l, input)
		}
		return nil
	default:
		return ErrLinkTerminal
	}
}
