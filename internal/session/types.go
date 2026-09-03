package session

import "errors"

// Common domain errors.
var (
	ErrSessionEnded   = errors.New("voice session has ended")
	ErrLinkSuperseded = errors.New("client link has been superseded")
	ErrLinkDetached   = errors.New("client link has been detached")
	ErrLinkTerminal   = errors.New("client link is terminal")
)

// SessionID is a gateway-owned identifier for a Voice Session.
type SessionID string

// OpenRequest carries parameters for opening a Voice Session.
type OpenRequest struct{}

// SessionInput represents domain input sent from a Voice Client to a Voice Session.
type SessionInput interface {
	isSessionInput()
}

// ClientTextInput delivers conversational text from a Voice Client.
type ClientTextInput struct {
	Text string
}

func (ClientTextInput) isSessionInput() {}

// SessionOutput represents domain output emitted from a Voice Session to a Voice Client.
type SessionOutput interface {
	isSessionOutput()
}
