package peer

import (
	"context"
	"sync"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	ds "github.com/ipfs/go-datastore"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"

	discovery "github.com/libp2p/go-libp2p/core/discovery"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	routing "github.com/libp2p/go-libp2p/core/routing"
)

type BootstrapParams struct {
	Enable bool
	Peers  []peer.AddrInfo
}

type Node struct {
	ctx                 context.Context
	ctx_cancel          context.CancelFunc
	ephemeral_repo_path bool
	repo_path           string
	store               ds.Batching
	key                 crypto.PrivKey
	id                  peer.ID
	secret              pnet.PSK
	host                host.Host
	dht                 routing.Routing
	drouter             discovery.Discovery
	messaging           *pubsub.PubSub
	ipfs                *ipfslite.Peer
	peering             *PeeringService

	topicsMutex sync.Mutex
	topics      map[string]*pubsub.Topic
	closed      bool
}

func (p *Node) ID() peer.ID {
	return p.id
}

func (p *Node) Peering() *PeeringService {
	return p.peering
}

func (p *Node) Peer() host.Host {
	return p.host
}

func (p *Node) Messaging() *(pubsub.PubSub) {
	return p.messaging
}

func (p *Node) Store() ds.Batching {
	return p.store
}

func (p *Node) DAG() *ipfslite.Peer {
	return p.ipfs
}

func (p *Node) Discovery() discovery.Discovery {
	return p.drouter
}

func (p *Node) Context() context.Context {
	return p.ctx
}
