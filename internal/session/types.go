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

// OpenRequest carries parameters for opening or resuming a Voice Session.
type OpenRequest struct {
	Resume string
}

// SessionInput represents domain input sent from a Voice Client to a Voice Session.
type SessionInput interface {
	isSessionInput()
}

// ClientAudioInput delivers audio data from a Voice Client.
type ClientAudioInput struct {
	Data []byte
}

func (ClientAudioInput) isSessionInput() {}

// ClientTextInput delivers conversational text from a Voice Client.
type ClientTextInput struct {
	Text string
}

func (ClientTextInput) isSessionInput() {}

// SessionOutput represents domain output emitted from a Voice Session to a Voice Client.
type SessionOutput interface {
	isSessionOutput()
}

// AudioOutput delivers synthesized audio to a Voice Client.
type AudioOutput struct {
	Data []byte
}

func (AudioOutput) isSessionOutput() {}

// TextOutput delivers conversational text to a Voice Client.
type TextOutput struct {
	Text string
}

func (TextOutput) isSessionOutput() {}

// SessionEndedOutput signals that the Voice Session has terminated.
type SessionEndedOutput struct {
	Reason string
}

func (SessionEndedOutput) isSessionOutput() {}
