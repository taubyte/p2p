package httptun

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/taubyte/p2p/streams/packer"
)

var (
	Magic   = packer.Magic{0x02, 0xfc}
	Version = packer.Version(0x01)
)

const (
	HeadersOp packer.Channel = 1
	RequestOp packer.Channel = 8
	BodyOp    packer.Channel = 16
)

var (
	DefaultBufferSize = 4 * 1024

	BodyStreamBufferSize = 1024

	ErrNotBody = errors.New("payload not body")
)

type bodyReader struct {
	packer packer.Packer
	ch     packer.Channel
	pre    io.Reader
	err    error
	stream io.Reader
}

func newBodyReader(p packer.Packer, ch packer.Channel, strm io.Reader) io.ReadCloser {
	return &bodyReader{
		packer: p,
		ch:     ch,
		stream: strm,
	}
}

func (b *bodyReader) Close() error {
	return nil
}

func (b *bodyReader) Read(p []byte) (n int, err error) {
	if b.err != nil {
		n, err = b.pre.Read(p)
		if err != nil { // only EOF is possible here
			err = b.err
		}
		return
	}

	if b.pre != nil {
		n, err = b.pre.Read(p)
		if n > 0 || err == nil {
			return
		}
	}

	var (
		ch packer.Channel
		l  int64
	)
	ch, l, err = b.packer.Next(b.stream)
	if err != nil {
		return
	}

	if ch != b.ch {
		var p [512]byte
		r := io.LimitReader(b.stream, l)
		for {
			_, _err := r.Read(p[:])
			if _err != nil {
				break
			}
		}
		return 0, ErrNotBody
	}

	b.pre = io.LimitReader(b.stream, l)
	n, err = b.pre.Read(p)

	return
}

// Stream -> HTTP
func Backend(stream io.ReadWriter) (http.ResponseWriter, *http.Request, error) {
	pack := packer.New(Magic, Version)

	var rpbuf bytes.Buffer
	ch, _, err := pack.Recv(stream, &rpbuf)
	if err != nil {
		return nil, nil, fmt.Errorf("reading http request failed with %w", err)
	}

	if ch != RequestOp {
		return nil, nil, errors.New("expected request payload")
	}

	var rpayload requestPayload

	err = rpayload.Decode(rpbuf.Bytes())
	if err != nil {
		return nil, nil, err
	}

	req, err := payloadToRequest(&rpayload, stream)
	if err != nil {
		return nil, nil, err
	}

	return newResponseWriter(stream), req, nil

}

// Note: make sure you call
// HTTP -> Stream
func Frontend(w http.ResponseWriter, r *http.Request, stream io.ReadWriter) error {
	var (
		wg        sync.WaitGroup
		exitError error
	)

	cont := true
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := requestToStream(stream, r)
		if err != nil {
			cont = false
			exitError = fmt.Errorf("request stream failed with %w", err)
			return
		}
	}()

	go func() {
		defer wg.Done()
		pack := packer.New(Magic, Version)

		for cont {
			ch, n, err := pack.Next(stream)
			if err != nil {
				if err == io.EOF {
					err = nil
				} else {
					exitError = fmt.Errorf("reading stream failed with %w", err)
				}
				return
			}

			payload := io.LimitReader(stream, n)
			var m int64
			switch ch {
			case HeadersOp:
				err = headersOp(w, payload)
			case BodyOp:
				m, err = bodyOp(w, payload)
				if m != n {
					err = errors.New("failed to forward body")
				}
			default:
				err = errors.New("failed to process http response op")
			}
			if err != nil {
				exitError = err
				return
			}
		}
	}()

	wg.Wait()

	return exitError
}

func requestToStream(stream io.Writer, r *http.Request) error {
	pack := packer.New(Magic, Version)

	rpayload := requestToPayload(r)

	rpbuf, err := rpayload.Encode()
	if err != nil {
		return err
	}

	err = pack.Send(RequestOp, stream, bytes.NewBuffer(rpbuf), int64(len(rpbuf)))
	if err != nil {
		return err
	}

	if r.Body != nil {
		defer r.Body.Close()
		buf := make([]byte, BodyStreamBufferSize)
		err = pack.Stream(BodyOp, stream, r.Body, buf)
		if err != io.EOF {
			return err
		}
	}

	return nil
}

func headersOp(w http.ResponseWriter, r io.Reader) error {
	dec := cbor.NewDecoder(r)
	var obj headersOpPayload
	err := dec.Decode(&obj)
	if err != nil {
		return err
	}

	curHeader := w.Header()

	// delete
	for k := range curHeader {
		if _, ok := obj.Headers[k]; !ok {
			curHeader.Del(k)
		}
	}

	// add/set
	for k, v := range obj.Headers {
		for i := 0; i < len(v); i++ {
			if i == 0 {
				curHeader.Set(k, v[i])
			} else {
				curHeader.Add(k, v[i])
			}
		}
	}

	w.WriteHeader(int(obj.Code))

	return nil
}

func bodyOp(w http.ResponseWriter, r io.Reader) (int64, error) {
	return io.Copy(w, r)
}
