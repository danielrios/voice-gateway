package session

import (
	"context"
	"sync"
)

// DefaultMediaQueueCapacity is the default bound for queued media chunks.
const DefaultMediaQueueCapacity = 32

type linkState int

const (
	linkActive linkState = iota
	linkDetached
	linkSuperseded
	linkTerminated
)

type linkItem struct {
	seq     uint64
	output  SessionOutput
	isMedia bool
	epoch   uint64
}

type clientLink struct {
	events   chan SessionOutput
	mu       sync.RWMutex
	state    linkState
	onDetach func(*clientLink)
	onSend   func(context.Context, *clientLink, SessionInput) error

	done     chan struct{}
	wakeChan chan struct{}

	queueMu           sync.Mutex
	queueCond         *sync.Cond
	queue             []linkItem
	nextSeq           uint64
	maxMediaQueue     int
	mediaCount        int
	invalidatedEpoch  uint64
	droppedMediaCount int64
}

func newClientLink(onDetach func(*clientLink), onSend func(context.Context, *clientLink, SessionInput) error) *clientLink {
	l := &clientLink{
		events:        make(chan SessionOutput),
		state:         linkActive,
		onDetach:      onDetach,
		onSend:        onSend,
		done:          make(chan struct{}),
		wakeChan:      make(chan struct{}, 1),
		maxMediaQueue: DefaultMediaQueueCapacity,
	}
	l.queueCond = sync.NewCond(&l.queueMu)
	go l.dispatchLoop()
	return l
}

func (l *clientLink) Events() <-chan SessionOutput {
	return l.events
}

func (l *clientLink) dispatchLoop() {
	defer close(l.events)

	for {
		select {
		case <-l.done:
			return
		default:
		}

		l.queueMu.Lock()
		l.purgeInvalidatedLocked()

		for len(l.queue) == 0 {
			l.queueMu.Unlock()
			select {
			case <-l.done:
				return
			case <-l.wakeChan:
			}
			l.queueMu.Lock()
			l.purgeInvalidatedLocked()
		}

		// Fast path: if a receiver is already waiting, deliver immediately under the lock.
		select {
		case l.events <- l.queue[0].output:
			if l.queue[0].isMedia {
				l.mediaCount--
			}
			l.queue[0] = linkItem{}
			l.queue = l.queue[1:]
			l.queueMu.Unlock()
			continue
		default:
		}

		nextItem := l.queue[0]
		l.queueMu.Unlock()

		select {
		case <-l.wakeChan:
			continue
		default:
		}

		select {
		case <-l.done:
			return
		case <-l.wakeChan:
			continue
		case l.events <- nextItem.output:
			l.queueMu.Lock()
			if len(l.queue) > 0 && l.queue[0].seq == nextItem.seq {
				if l.queue[0].isMedia {
					l.mediaCount--
				}
				l.queue[0] = linkItem{}
				l.queue = l.queue[1:]
			} else {
				for i, item := range l.queue {
					if item.seq == nextItem.seq {
						if item.isMedia {
							l.mediaCount--
						}
						copy(l.queue[i:], l.queue[i+1:])
						l.queue[len(l.queue)-1] = linkItem{}
						l.queue = l.queue[:len(l.queue)-1]
						break
					}
				}
			}
			l.queueMu.Unlock()
		}
	}
}

func (l *clientLink) enqueueControl(output SessionOutput) {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()

	l.nextSeq++
	item := linkItem{seq: l.nextSeq, output: output, isMedia: false}
	l.queue = append(l.queue, item)
	l.notifyWake()
}

func (l *clientLink) enqueueMedia(output SessionOutput, epoch uint64) {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()

	if epoch <= l.invalidatedEpoch {
		l.droppedMediaCount++
		if l.queueCond != nil {
			l.queueCond.Broadcast()
		}
		return
	}

	// Bounded queue: drop oldest media chunk to prioritize realtime freshness under backpressure.
	if l.mediaCount >= l.maxMediaQueue {
		for i, item := range l.queue {
			if item.isMedia {
				copy(l.queue[i:], l.queue[i+1:])
				l.queue[len(l.queue)-1] = linkItem{}
				l.queue = l.queue[:len(l.queue)-1]
				l.mediaCount--
				l.droppedMediaCount++
				if l.queueCond != nil {
					l.queueCond.Broadcast()
				}
				break
			}
		}
	}

	l.nextSeq++
	l.queue = append(l.queue, linkItem{seq: l.nextSeq, output: output, isMedia: true, epoch: epoch})
	l.mediaCount++
	l.notifyWake()
}

func (l *clientLink) invalidatePlayback(epoch uint64) {
	l.queueMu.Lock()
	if epoch > l.invalidatedEpoch {
		l.invalidatedEpoch = epoch
	}
	l.purgeInvalidatedLocked()
	l.queueMu.Unlock()
	l.notifyWake()
}

func (l *clientLink) purgeInvalidatedLocked() {
	if l.invalidatedEpoch == 0 {
		return
	}
	changed := false
	newQ := l.queue[:0]
	for _, item := range l.queue {
		if item.isMedia && item.epoch <= l.invalidatedEpoch {
			l.mediaCount--
			l.droppedMediaCount++
			changed = true
		} else {
			newQ = append(newQ, item)
		}
	}
	for i := len(newQ); i < len(l.queue); i++ {
		l.queue[i] = linkItem{}
	}
	l.queue = newQ
	if changed && l.queueCond != nil {
		l.queueCond.Broadcast()
	}
}

func (l *clientLink) notifyWake() {
	select {
	case l.wakeChan <- struct{}{}:
	default:
	}
}

func (l *clientLink) DroppedMediaCount() int64 {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	return l.droppedMediaCount
}

func (l *clientLink) WaitForDroppedMediaCount(target int64) int64 {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	for l.droppedMediaCount < target {
		select {
		case <-l.done:
			return l.droppedMediaCount
		default:
		}
		l.queueCond.Wait()
	}
	return l.droppedMediaCount
}

func (l *clientLink) SetMaxMediaQueue(capacity int) {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if capacity > 0 {
		l.maxMediaQueue = capacity
	}
}

func (l *clientLink) closeWithState(newState linkState) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.state == linkActive {
		l.state = newState
		close(l.done)
		l.queueMu.Lock()
		if l.queueCond != nil {
			l.queueCond.Broadcast()
		}
		l.queueMu.Unlock()
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
