package session

import (
	"context"
	"sync"
	"sync/atomic"
)

type linkState int32

const (
	linkActive linkState = iota
	linkDetached
	linkSuperseded
	linkTerminated
)

type clientLink struct {
	events   chan SessionOutput
	state    atomic.Int32
	detachMu sync.Mutex
	onDetach func(*clientLink)
	onSend   func(context.Context, SessionInput) error
}

func newClientLink(onDetach func(*clientLink), onSend func(context.Context, SessionInput) error) *clientLink {
	return &clientLink{
		events:   make(chan SessionOutput, 64),
		onDetach: onDetach,
		onSend:   onSend,
	}
}

func (l *clientLink) Events() <-chan SessionOutput {
	return l.events
}

func (l *clientLink) Detach() error {
	l.detachMu.Lock()
	defer l.detachMu.Unlock()

	if l.state.CompareAndSwap(int32(linkActive), int32(linkDetached)) {
		close(l.events)
		if l.onDetach != nil {
			l.onDetach(l)
		}
	}
	return nil
}

func (l *clientLink) supersede() {
	l.detachMu.Lock()
	defer l.detachMu.Unlock()

	if l.state.CompareAndSwap(int32(linkActive), int32(linkSuperseded)) {
		close(l.events)
	}
}

func (l *clientLink) terminate() {
	l.detachMu.Lock()
	defer l.detachMu.Unlock()

	if l.state.CompareAndSwap(int32(linkActive), int32(linkTerminated)) {
		close(l.events)
	}
}

func (l *clientLink) Send(ctx context.Context, input SessionInput) error {
	switch linkState(l.state.Load()) {
	case linkDetached:
		return ErrLinkDetached
	case linkSuperseded:
		return ErrLinkSuperseded
	case linkTerminated:
		return ErrLinkTerminal
	case linkActive:
		if l.onSend != nil {
			return l.onSend(ctx, input)
		}
		return nil
	default:
		return ErrLinkTerminal
	}
}
