package httptun

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

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
	BodyStreamBufferSize = 4 * 1024

	ErrNotBody = errors.New("payload not an http body")
)

type bodyReader struct {
	packer packer.Packer
	ch     packer.Channel
	pre    io.Reader
	err    error
	stream io.Reader
	len    int
}

func newBodyReader(p packer.Packer, ch packer.Channel, strm io.Reader) io.ReadCloser {
	return &bodyReader{
		packer: p,
		ch:     ch,
		stream: strm,
	}
}

func (b *bodyReader) Close() error {
	//TODO: send close to frontend
	return nil
}

func (b *bodyReader) Read(p []byte) (n int, err error) {
	defer func() {
		b.len += n
	}()

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
		b.err = err
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
		exitError error
	)

	cont := true
	done := make(chan struct{})

	go func() {
		defer func() {
			done <- struct{}{}
		}()
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

	_, _, err := requestToStream(stream, r)
	if err != nil {
		cont = false
		exitError = fmt.Errorf("request stream failed with %w", err)
		return exitError
	}

	<-done

	return exitError
}

func requestToStream(stream io.Writer, r *http.Request) (int64, int64, error) {
	pack := packer.New(Magic, Version)

	rpayload := requestToPayload(r)

	rpbuf, err := rpayload.Encode()
	if err != nil {
		return 0, 0, err
	}

	hdrlen := int64(len(rpbuf))
	err = pack.Send(RequestOp, stream, bytes.NewBuffer(rpbuf), hdrlen)
	if err != nil {
		return 0, 0, err
	}

	bodylen, _ := pack.Stream(BodyOp, stream, r.Body, BodyStreamBufferSize)
	r.Body.Close()

	return hdrlen, bodylen, nil
}

func headersOp(w http.ResponseWriter, r io.Reader) error {
	dec := cbor.NewDecoder(r)
	var obj headersOpPayload
	err := dec.Decode(&obj)
	if err != nil {
		return err
	}

	// delete
	for k := range w.Header() {
		if _, ok := obj.Headers[k]; !ok {
			w.Header().Del(k)
		}
	}

	// add/set
	for k, v := range obj.Headers {
		for i := 0; i < len(v); i++ {
			if i == 0 {
				w.Header().Set(k, v[i])
			} else {
				w.Header().Add(k, v[i])
			}
		}
	}

	w.WriteHeader(int(obj.Code))

	return nil
}

func bodyOp(w http.ResponseWriter, r io.Reader) (int64, error) {
	return io.Copy(w, r)
}
