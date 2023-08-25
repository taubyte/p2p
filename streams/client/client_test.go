package client

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	keypair "github.com/taubyte/p2p/keypair"

	peer "github.com/taubyte/p2p/peer"
	"github.com/taubyte/p2p/streams/command"
	peerService "github.com/taubyte/p2p/streams/service"

	"github.com/taubyte/p2p/streams"
	cr "github.com/taubyte/p2p/streams/command/response"

	logging "github.com/ipfs/go-log/v2"
	peercore "github.com/libp2p/go-libp2p/core/peer"
)

func TestClientSend(t *testing.T) {
	logging.SetLogLevel("*", "error")

	ctx, ctxC := context.WithCancel(context.Background())
	defer ctxC()

	rand.Seed(time.Now().UnixNano())

	var n int
	for n < 25565 || n > 40000 {
		n = rand.Intn(100000)
	}

	p1, err := peer.New( // provider
		ctx,
		nil,
		keypair.NewRaw(),
		nil,
		[]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", n)},
		nil,
		true,
		false,
	)
	if err != nil {
		t.Errorf("Peer creation returned error `%s`", err.Error())
		return
	}
	defer p1.Close()

	svr, err := peerService.New(p1, "hello", "/hello/1.0")
	if err != nil {
		t.Errorf("Service creation returned error `%s`", err.Error())
		return
	}
	defer svr.Stop()
	err = svr.Define("hi", func(context.Context, streams.Connection, command.Body) (cr.Response, error) {
		return cr.Response{"message": "HI"}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	err = svr.Define("echo", func(_ctx context.Context, _ streams.Connection, _body command.Body) (cr.Response, error) {
		return cr.Response{"message": _body["message"].(string)}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	p2, err := peer.New( // consumer
		ctx,
		nil,
		keypair.NewRaw(),
		nil,
		[]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", n+1)},
		nil,
		true,
		false,
	)
	if err != nil {
		t.Errorf("Ping test returned error `%s`", err.Error())
		return
	}
	defer p2.Close()

	err = p2.Peer().Connect(ctx, peercore.AddrInfo{ID: p1.ID(), Addrs: p1.Peer().Addrs()})
	if err != nil {
		t.Errorf("Connect to peer %v returned `%s`", p1.Peer().Addrs(), err.Error())
		return
	}

	// static peers
	c, err := New(ctx, p2, []string{p1.ID().String()}, "/hello/1.0", 1, 1)
	if err != nil {
		t.Errorf("Client creation returned error `%s`", err.Error())
	} else {
		// no arg command
		res, err := c.Send("hi", command.Body{})
		if err != nil {
			t.Errorf("Sending command returned error `%s`", err.Error())
			return
		} else if v, k := res["message"]; k == false || v.(string) != "HI" {
			t.Errorf("Provider response does not match %v", res)
			return
		}

		// command with argument
		res, err = c.Send("echo", command.Body{"message": "back"})
		if err != nil {
			t.Errorf("Sending command returned error `%s`", err.Error())
			return
		} else if v, k := res["message"]; k == false || v.(string) != "back" {
			t.Errorf("Provider response does not match %v", res)
			return
		}

		// command with big argument
		bigMessageBase := "1234567890qwertyuiopasdfghjklzxcvbnm1234567890qwertyuiopasdfghjklzxcvbnm"
		var bigMessage string
		bigMessageCount := 1024 * 1024 / len(bigMessageBase)
		for i := 0; i < bigMessageCount; i++ {
			bigMessage += bigMessageBase
		}
		res, err = c.Send("echo", command.Body{"message": bigMessage})
		if err != nil {
			t.Errorf("Sending command returned error `%s`", err.Error())
			return
		} else if v, k := res["message"]; k == false || v.(string) != bigMessage {
			t.Errorf("Provider response does not match %v", res)
			return
		}

		//invalid command
		_, err = c.Send("notExist", command.Body{})
		if err == nil {
			t.Error("Non existing command not handled correctly")
			return
		}

		// Close
		c.Close()
	}

	// discover
	cd, err := New(ctx, p2, nil, "/hello/1.0", 1, 1)
	if err != nil {
		t.Errorf("Client creation returned error `%s`", err.Error())
		return
	} else {
		res, err := cd.Send("hi", command.Body{})
		if err != nil {
			t.Errorf("Sending command returned error `%s`", err.Error())
			return
		} else if v, k := res["message"]; k == false || v.(string) != "HI" {
			t.Errorf("Provider response does not match %#v", res)
			return
		}

		//Close
		cd.Close()
	}
}

func TestClientMultiSend(t *testing.T) {
	logging.SetLogLevel("*", "error")

	ctx, ctxC := context.WithCancel(context.Background())
	defer ctxC()

	rand.Seed(time.Now().UnixNano())

	var n int
	for n < 25565 || n > 40000 {
		n = rand.Intn(100000)
	}

	p1, err := peer.New( // provider
		ctx,
		nil,
		keypair.NewRaw(),
		nil,
		[]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", n)},
		nil,
		true,
		false,
	)
	if err != nil {
		t.Errorf("Peer creation returned error `%s`", err.Error())
		return
	}
	defer p1.Close()

	svr1, err := peerService.New(p1, "hello", "/hello/1.0")
	if err != nil {
		t.Errorf("Service creation returned error `%s`", err.Error())
		return
	}
	defer svr1.Stop()
	err = svr1.Define("hi", func(context.Context, streams.Connection, command.Body) (cr.Response, error) {
		return cr.Response{"message": "HI"}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	p2, err := peer.New( // provider
		ctx,
		nil,
		keypair.NewRaw(),
		nil,
		[]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", n)},
		nil,
		true,
		false,
	)
	if err != nil {
		t.Errorf("Peer creation returned error `%s`", err.Error())
		return
	}
	defer p2.Close()

	svr2, err := peerService.New(p2, "hello", "/hello/1.0")
	if err != nil {
		t.Errorf("Service creation returned error `%s`", err.Error())
		return
	}
	defer svr2.Stop()
	err = svr2.Define("hi", func(context.Context, streams.Connection, command.Body) (cr.Response, error) {
		return cr.Response{"message": "HI"}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	p3, err := peer.New( // consumer
		ctx,
		nil,
		keypair.NewRaw(),
		nil,
		[]string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", n+1)},
		nil,
		true,
		false,
	)
	if err != nil {
		t.Errorf("Ping test returned error `%s`", err.Error())
		return
	}
	defer p3.Close()

	err = p3.Peer().Connect(ctx, peercore.AddrInfo{ID: p1.ID(), Addrs: p1.Peer().Addrs()})
	if err != nil {
		t.Errorf("Connect to peer %v returned `%s`", p1.Peer().Addrs(), err.Error())
		return
	}

	err = p3.Peer().Connect(ctx, peercore.AddrInfo{ID: p2.ID(), Addrs: p2.Peer().Addrs()})
	if err != nil {
		t.Errorf("Connect to peer %v returned `%s`", p2.Peer().Addrs(), err.Error())
		return
	}

	// discover
	cd, err := New(ctx, p3, nil, "/hello/1.0", 2, 2)
	if err != nil {
		t.Errorf("Client creation returned error `%s`", err.Error())
		return
	} else {
		res, errs, err := cd.MultiSend("hi", command.Body{}, 2)
		if err != nil {
			t.Errorf("Sending command returned error `%s`", err.Error())
			return
		}

		if len(res) != 2 && len(errs) == 0 {
			t.Errorf("MultiSending command failed R=%d, E=%d", len(res), len(errs))
			return
		}

		for p, r := range res {
			if r["message"] != "HI" {
				t.Errorf("node %s returned bad response `%s`", p.Pretty(), r)
				return
			}
		}

		//Close
		cd.Close()
	}
}
