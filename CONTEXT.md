# Voice Gateway

Canonical language for the realtime voice gateway domain.

## Language

**Voice Client**:
A device or application that participates in a realtime spoken interaction through the gateway.
_Avoid_: Device, frontend, endpoint

**Voice Session**:
A continuous conversational interaction that binds one Voice Client to one Voice Provider and may outlive an individual network connection.
_Avoid_: Connection, socket, call

**Voice Provider**:
An external realtime model that consumes conversational input and produces conversational output, including audio, text, and tool requests.
_Avoid_: LLM, model backend, TTS provider

**Agent Runtime**:
A system that owns agentic work such as tools, memory, coding, research, files, approvals, and long-running tasks.
_Avoid_: Brain, worker service, agent backend

**Delegation**:
A request from a Voice Session for an Agent Runtime to perform agentic work independently of the realtime conversational turn.
_Avoid_: Tool call, job

**Interaction**:
A request from an Agent Runtime that requires user input before delegated work can continue, such as clarification or approval.
_Avoid_: Prompt, callback

**Announcement**:
A gateway-originated conversational update injected into a Voice Session because an external event became relevant, commonly completion or failure of a Delegation.
_Avoid_: Notification, message

**Turn**:
One conversational exchange lifecycle, including input, provider generation, tool waiting, playback, completion, or interruption.
_Avoid_: Request, response
