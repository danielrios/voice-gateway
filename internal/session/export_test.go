package session

import "io"

// SetRandReaderForTesting allows testing entropy failure in session_test.
func SetRandReaderForTesting(r io.Reader) func() {
	old := randReader
	randReader = r
	return func() {
		randReader = old
	}
}
