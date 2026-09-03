package session

import (
	"context"
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

type handle struct {
	id       SessionID
	provider ProviderSession
	runtime  RuntimeSession

	ctx     context.Context
	cancel  context.CancelFunc
	cmdChan chan any
	done    chan struct{}
}

func newHandle(id SessionID, provider ProviderSession, runtime RuntimeSession) *handle {
	ctx, cancel := context.WithCancel(context.Background())
	h := &handle{
		id:       id,
		provider: provider,
		runtime:  runtime,
		ctx:      ctx,
		cancel:   cancel,
		cmdChan:  make(chan any, 64),
		done:     make(chan struct{}),
	}
	go h.run()
	return h
}

func (h *handle) ID() SessionID {
	return h.id
}

func (h *handle) run() {
	var activeLink *clientLink

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

	for {
		select {
		case <-h.ctx.Done():
			cleanup()
			close(h.done)
			drainCommands()
			return

		case msg := <-h.cmdChan:
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
				if activeLink != cmd.link {
					cmd.reply <- ErrLinkSuperseded
					continue
				}
				cmd.reply <- nil

			case endCmd:
				cleanup()
				close(h.done)
				drainCommands()
				cmd.reply <- nil
				return
			}
		}
	}
}

func (h *handle) Attach(ctx context.Context) (ClientLink, error) {
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
