package client

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	"github.com/ipfs/go-cid"
	"github.com/taubyte/p2p/peer"
	cr "github.com/taubyte/p2p/streams/command/response"

	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/network"
	peerCore "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"

	log "github.com/ipfs/go-log/v2"

	"github.com/taubyte/p2p/streams/command"
)

var (
	NumConnectTries        int           = 3
	NumStreamers           int           = 3
	DiscoveryLimit         int           = 1024
	SendTimeout            time.Duration = 3 * time.Second
	RecvTimeout            time.Duration = 3 * time.Second
	EstablishStreamTimeout time.Duration = 5 * time.Second
	SendToPeerTimeout      time.Duration = 10 * time.Second
)
var (
	logger log.StandardLogger
)

func init() {
	logger = log.Logger("p2p.streams.client")
}

type Client struct {
	ctx  context.Context
	ctxC context.CancelFunc
	node peer.Node
	path string
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func New(ctx context.Context, node peer.Node, peers []string, path string, min int, max int) (*Client, error) {
	c := &Client{
		node: node,
		path: path,
	}

	c.ctx, c.ctxC = context.WithCancel(ctx)

	return c, nil
}

func (c *Client) SendTo(cid cid.Cid, cmdName string, body command.Body) (cr.Response, error) {
	cmd := command.New(cmdName, body)
	pid, err := peerCore.FromCid(cid)
	if err != nil {
		return nil, fmt.Errorf("decoding peer id failed with: %w", err)
	}

	ctx, ctx_close := context.WithTimeout(c.ctx, SendToPeerTimeout)
	now := time.Now()
	defer ctx_close()

	strm, err := c.node.Peer().NewStream(ctx, pid, protocol.ID(c.path))
	if err != nil {
		return nil, fmt.Errorf("peer new stream failed with: %w", err)
	}

	defer strm.Reset()

	if err = strm.SetWriteDeadline(now.Add(SendTimeout)); err != nil {
		return nil, fmt.Errorf("set write deadline failed with: %w", err)
	}

	if err = cmd.Encode(strm); err != nil {
		return nil, fmt.Errorf("encoding command failed with: %w", err)
	}

	if err = strm.SetReadDeadline(now.Add(RecvTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline failed with: %w", err)
	}

	response, err := cr.Decode(strm)
	if err != nil {
		return nil, fmt.Errorf("decoding stream failed with: %w", err)
	}

	if v, k := response["error"]; k {
		// return non nil response so we now it's not a network error
		return cr.Response{}, errors.New(fmt.Sprint(v))
	}

	return response, nil
}

func min(t0 time.Time, t1 time.Time) time.Time {
	if t0.Before(t1) {
		return t0
	}
	return t1
}

func (c *Client) Send(cmdName string, body command.Body) (cr.Response, error) {
	_, response, err := c.send(cmdName, body)
	return response, err
}

func (c *Client) SendForPid(cmdName string, body command.Body) (peerCore.ID, cr.Response, error) {
	return c.send(cmdName, body)
}

func (c *Client) send(cmdName string, body command.Body) (peerCore.ID, cr.Response, error) {
	cmd := command.New(cmdName, body)
	now := time.Now()
	ctx, ctx_close := context.WithDeadline(c.ctx, now.Add(SendToPeerTimeout))
	defer ctx_close()
	strmDD, _ := ctx.Deadline()

	storedPeers := c.node.Peer().Peerstore().Peers()

	cap := 32
	if len(storedPeers) > cap {
		cap = len(storedPeers)
	}

	peers := make(chan peerCore.AddrInfo, cap)

	proto := protocol.ID(c.path)

	for _, peer := range storedPeers {
		if len(peer) == 0 || c.node.ID() == peer {
			continue
		}

		protos, err := c.node.Peer().Peerstore().GetProtocols(peer)
		if err != nil {
			logger.Errorf("getting protocols for `%s` failed with: %s", peer, err)
			continue
		}

		if slices.Contains(protos, proto) {
			peers <- peerCore.AddrInfo{ID: peer, Addrs: c.node.Peer().Peerstore().Addrs(peer)}
		}
	}

	if len(peers) == 0 {
		discPeers, err := c.node.Discovery().FindPeers(ctx, c.path, discovery.Limit(DiscoveryLimit))
		if err != nil {
			return "", nil, err
		}

		go func() {
			defer close(peers)

			for {
				select {
				case <-ctx.Done():
					return
				case peer := <-discPeers:
					if len(peer.ID) > 0 && peer.ID != c.node.ID() {
						if len(peer.Addrs) == 0 {
							peer.Addrs = c.node.Peer().Peerstore().Addrs(peer.ID)
						}
						if len(peer.Addrs) > 0 {
							peers <- peer
						}
					}
				}
			}
		}()
	}

	var (
		strm network.Stream
		pid  peerCore.ID
	)
	for strm == nil {
		select {
		case peer, ok := <-peers:
			if !ok {
				return "", nil, errors.New("peer discovery channel closed")
			}

			pid = peer.ID

			repush := true
			switch c.node.Peer().Network().Connectedness(pid) {
			case network.Connected:
				repush = false
			case network.CannotConnect:
				repush = false
				continue
			case network.CanConnect, network.NotConnected:
				go c.node.Peer().Connect(network.WithDialPeerTimeout(c.ctx, time.Until(strmDD)), peer)
			default:
				repush = false
				continue
			}

			if repush {
				// push it back
				select {
				case peers <- peer:
				default:
				}
				continue
			}

			var err error
			strm, err = c.node.Peer().NewStream(network.WithNoDial(c.ctx, "application ensured connection to peer exists"), pid, proto)
			if err != nil {
				logger.Errorf("starting stream to `%s`;`%s` failed with: %s", pid.String(), c.path, err)
				continue
			}
		case <-ctx.Done():
			return "", nil, fmt.Errorf("%s: finding peers for command `%s`.`%s` timed out!: %s", c.node.ID(), c.path, cmdName, ctx.Err())
		}
	}

	defer strm.Close()

	if err := strm.SetWriteDeadline(min(time.Now().Add(SendTimeout), strmDD)); err != nil {
		return "", nil, fmt.Errorf("setting write deadline at `%s`;`%s` failed with: %s", pid.String(), c.path, err)
	}

	if err := cmd.Encode(strm); err != nil {
		dl, ok := ctx.Deadline()
		return "", nil, fmt.Errorf("encoding at `%s`;`%s` t(%s::%v) failed with: %s", pid.String(), c.path, dl.String(), ok, err)
	}

	if err := strm.SetReadDeadline(min(time.Now().Add(RecvTimeout), strmDD)); err != nil {
		return "", nil, fmt.Errorf("setting read deadline at `%s`;`%s` failed with: %s", pid.String(), c.path, err)
	}

	response, err := cr.Decode(strm)
	if err != nil {
		dl, ok := ctx.Deadline()
		return "", nil, fmt.Errorf("decoding at `%s`;`%s` t(%s::%v) failed with: %s", pid.String(), c.path, dl.String(), ok, err)
	}

	if v, k := response["error"]; k {
		return pid, cr.Response{}, errors.New(fmt.Sprint(v))
	}

	return pid, response, nil
}

// this command will only fail with a timeout error or out of peers
func (c *Client) TrySend(cmdName string, body command.Body) (cr.Response, error) {
	return c.Send(cmdName, body)
}

func (c *Client) Close() {
	c.ctxC()
}
