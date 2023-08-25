package client

import (
	"context"
	"fmt"
	"sync"
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

type Client struct {
	ctx  context.Context
	ctxC context.CancelFunc
	node peer.Node
	path string
}

type response struct {
	peerCore.ID
	cr.Response
	error
}

type stream struct {
	network.Stream
	peerCore.ID
}

var (
	NumConnectTries        int           = 3
	NumStreamers           int           = 3
	DiscoveryLimit         int           = 1024
	SendTimeout            time.Duration = 3 * time.Second
	RecvTimeout            time.Duration = 3 * time.Second
	EstablishStreamTimeout time.Duration = 5 * time.Second
	SendToPeerTimeout      time.Duration = 10 * time.Second

	logger log.StandardLogger
)

func init() {
	logger = log.Logger("p2p.streams.client")
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
	responses, err := c.send(cmdName, body, 1)
	if err != nil {
		return nil, err
	}

	resp, ok := <-responses
	if !ok {
		return nil, errors.New("timeout")
	}

	if resp.error != nil {
		return nil, resp.error
	}

	return resp.Response, nil
}

func (c *Client) MultiSend(cmdName string, body command.Body, thresh int) (map[peerCore.ID]cr.Response, map[peerCore.ID]error, error) {
	responses, err := c.send(cmdName, body, thresh)
	if err != nil {
		return nil, nil, err
	}
	rets := make(map[peerCore.ID]cr.Response)
	errs := make(map[peerCore.ID]error)
	for resp := range responses {
		if resp.error != nil {
			errs[resp.ID] = resp.error
		} else {
			rets[resp.ID] = resp.Response
		}
	}

	return rets, errs, nil
}

func (c *Client) discover(ctx context.Context) <-chan peerCore.AddrInfo {
	storedPeers := c.node.Peer().Peerstore().Peers()
	cap := 32
	if len(storedPeers) > cap {
		cap = len(storedPeers)
	}

	peers := make(chan peerCore.AddrInfo, cap)

	go func() {
		defer close(peers)
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
				logger.Errorf("discovering nodes for `%s` failed with: %s", proto, err)
				return
			}

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
		}
	}()

	return peers
}

func (c *Client) connect(peer peerCore.AddrInfo) (network.Stream, bool, error) {
	switch c.node.Peer().Network().Connectedness(peer.ID) {
	case network.Connected:
	case network.CanConnect, network.NotConnected:
		go c.node.Peer().Connect(c.ctx, peer)
		return nil, true, nil
	default:
		return nil, false, nil
	}

	strm, err := c.node.Peer().NewStream(network.WithNoDial(c.ctx, "application ensured connection to peer exists"), peer.ID, protocol.ID(c.path))
	if err != nil {
		logger.Errorf("starting stream to `%s`;`%s` failed with: %s", peer.ID.String(), c.path, err)
		return nil, false, err
	}

	return strm, false, nil
}

func (c *Client) sendTo(strm stream, deadline time.Time, cmdName string, body command.Body) response {
	cmd := command.New(cmdName, body)

	if err := strm.SetWriteDeadline(min(time.Now().Add(SendTimeout), deadline)); err != nil {
		return response{
			ID:    strm.ID,
			error: fmt.Errorf("setting write deadline failed with: %s", err),
		}
	}

	if err := cmd.Encode(strm); err != nil {
		return response{
			ID:    strm.ID,
			error: fmt.Errorf("seding command `%s(%s)` failed with: %s", cmd.Command, c.path, err),
		}
	}

	if err := strm.SetReadDeadline(min(time.Now().Add(RecvTimeout), deadline)); err != nil {
		return response{
			ID:    strm.ID,
			error: fmt.Errorf("setting read deadline failed with: %s", err),
		}
	}

	resp, err := cr.Decode(strm)
	if err != nil {
		return response{
			ID:    strm.ID,
			error: fmt.Errorf("recv response of `%s(%s)` failed with: %s", cmd.Command, c.path, err),
		}
	}

	if v, k := resp["error"]; k {
		return response{
			ID:    strm.ID,
			error: errors.New(fmt.Sprint(v)),
		}
	}

	return response{
		ID:       strm.ID,
		Response: resp,
	}
}

func (c *Client) send(cmdName string, body command.Body, minStreams int) (<-chan response, error) {
	now := time.Now()
	ctx, _ := context.WithDeadline(c.ctx, now.Add(SendToPeerTimeout))
	strmDD, _ := ctx.Deadline()

	discPeers := c.discover(ctx)

	strms := make(chan stream, minStreams)
	strmsCount := 0
	go func() {
		defer close(strms)

		peers := make(chan peerCore.AddrInfo, 64)
		defer close(peers)

		for {
			if strmsCount >= minStreams {
				return
			}
			select {
			case peer, ok := <-discPeers:
				if ok {
					peers <- peer
				}
			case peer := <-peers:
				strm, repush, _ := c.connect(peer)
				if strm != nil && strmsCount < minStreams {
					strmsCount++
					strms <- stream{Stream: strm, ID: peer.ID}
				}
				if repush {
					peers <- peer
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	responses := make(chan response, minStreams)
	go func() {
		var wg sync.WaitGroup
		defer func() {
			wg.Wait()
			close(responses)
		}()
		for strm := range strms {
			wg.Add(1)
			go func(_strm stream) {
				defer wg.Done()
				responses <- c.sendTo(_strm, strmDD, cmdName, body)
			}(strm)
		}
	}()

	return responses, nil
}

// this command will only fail with a timeout error or out of peers
func (c *Client) TrySend(cmdName string, body command.Body) (cr.Response, error) {
	return c.Send(cmdName, body)
}

func (c *Client) Close() {
	c.ctxC()
}
