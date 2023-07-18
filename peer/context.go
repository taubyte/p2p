package peer

import "context"

func (p *Node) NewChildContextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.ctx)
}
