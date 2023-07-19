package mocks

import (
	"bytes"
	"context"
	"io"
	"sync"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/boxo/blockstore"
	"github.com/taubyte/p2p/peer"
)

type MockedNode interface{ peer.Node }

type mockNode struct {
	mapDef map[string][]byte
	lock   sync.RWMutex
	peer.Node
	context  context.Context
	contextC context.CancelFunc
}

type MockedReadSeekCloser interface{ peer.ReadSeekCloser }

type mockReadSeekCloser struct {
	*bytes.Buffer
	io.Seeker
	io.Writer
}

type MockedDag interface{ *ipfslite.Peer }

type MockedBlockStore interface{ blockstore.Blockstore }

type mockedBlockStore struct{ blockstore.Blockstore }