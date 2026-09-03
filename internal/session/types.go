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

// TurnID is a gateway-owned identifier for a Turn.
type TurnID string

// OpenRequest carries parameters for opening a Voice Session.
type OpenRequest struct{}

// SessionInput represents domain input sent from a Voice Client to a Voice Session.
type SessionInput interface {
	isSessionInput()
}

// ClientAudioInput delivers PCM audio data from a Voice Client.
type ClientAudioInput struct {
	Data []byte
}

func (ClientAudioInput) isSessionInput() {}

// ClientTextInput delivers conversational text from a Voice Client.
type ClientTextInput struct {
	Text string
}

func (ClientTextInput) isSessionInput() {}

// ClientInterruptedInput signals that the Voice Client interrupted the current Turn.
type ClientInterruptedInput struct{}

func (ClientInterruptedInput) isSessionInput() {}

// SessionOutput represents domain output emitted from a Voice Session to a Voice Client.
type SessionOutput interface {
	isSessionOutput()
}

// AudioOutput delivers synthesized audio to a Voice Client.
type AudioOutput struct {
	TurnID TurnID
	Data   []byte
}

func (AudioOutput) isSessionOutput() {}

// TextOutput delivers conversational text to a Voice Client.
type TextOutput struct {
	TurnID TurnID
	Text   string
}

func (TextOutput) isSessionOutput() {}

// TurnStartedOutput signals that a Turn has started.
type TurnStartedOutput struct {
	TurnID TurnID
}

func (TurnStartedOutput) isSessionOutput() {}

// TurnCompletedOutput signals that a Turn has completed.
type TurnCompletedOutput struct {
	TurnID TurnID
}

func (TurnCompletedOutput) isSessionOutput() {}

// TurnInterruptedOutput signals that a Turn was interrupted.
type TurnInterruptedOutput struct {
	TurnID TurnID
}

func (TurnInterruptedOutput) isSessionOutput() {}
