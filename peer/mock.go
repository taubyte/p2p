package peer

import "context"

func MockNode(ctx context.Context) *Node {
	n := &Node{}
	n.ctx, n.ctx_cancel = context.WithCancel(ctx)

	return n
}
