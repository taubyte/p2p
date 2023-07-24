package peer

import "context"

func MockNode(ctx context.Context) *node {
	n := &node{}
	n.ctx, n.ctx_cancel = context.WithCancel(ctx)

	return n
}
