package session

import (
	"io"
	"sync"
)

// SetRandReaderForTesting allows testing entropy failure in session_test.
func SetRandReaderForTesting(r io.Reader) func() {
	old := randReader
	randReader = r
	return func() {
		randReader = old
	}
}

// SetMediaQueueCapacityForTesting overrides the media queue capacity of a link for tests.
func SetMediaQueueCapacityForTesting(link ClientLink, capacity int) {
	if cl, ok := link.(*clientLink); ok {
		cl.SetMaxMediaQueue(capacity)
	}
}

// GetDroppedMediaCountForTesting returns the number of dropped media chunks for a link.
func GetDroppedMediaCountForTesting(link ClientLink) int64 {
	if cl, ok := link.(*clientLink); ok {
		return cl.DroppedMediaCount()
	}
	return 0
}

// WaitForDroppedMediaCountForTesting blocks until droppedMediaCount reaches target for a link.
func WaitForDroppedMediaCountForTesting(link ClientLink, target int64) int64 {
	if cl, ok := link.(*clientLink); ok {
		return cl.WaitForDroppedMediaCount(target)
	}
	return 0
}

// EnqueueMediaDirectForTesting directly enqueues a media chunk with a specified epoch.
func EnqueueMediaDirectForTesting(link ClientLink, output SessionOutput, epoch uint64) {
	if cl, ok := link.(*clientLink); ok {
		cl.enqueueMedia(output, epoch)
	}
}

// InvalidatePlaybackForTesting directly calls invalidatePlayback on a link.
func InvalidatePlaybackForTesting(link ClientLink, epoch uint64) {
	if cl, ok := link.(*clientLink); ok {
		cl.invalidatePlayback(epoch)
	}
}

// SetOnDispatchWaitingForTesting configures a hook called when dispatchLoop is about to wait on events send.
func SetOnDispatchWaitingForTesting(link ClientLink, fn func()) {
	if cl, ok := link.(*clientLink); ok {
		cl.queueMu.Lock()
		cl.onDispatchWaiting = fn
		cl.queueMu.Unlock()
	}
}

// WaitForMediaCountForTesting blocks until link's mediaCount equals the target count.
func WaitForMediaCountForTesting(link ClientLink, count int) {
	if cl, ok := link.(*clientLink); ok {
		cl.queueMu.Lock()
		defer cl.queueMu.Unlock()
		if cl.queueCond == nil {
			cl.queueCond = sync.NewCond(&cl.queueMu)
		}
		for cl.mediaCount != count {
			cl.queueCond.Wait()
		}
	}
}
