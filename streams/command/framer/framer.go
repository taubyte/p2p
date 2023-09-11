package framer

import (
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
)

type MessageHeader struct {
	Magic   [2]byte `cbor:"1,keyasint"`
	Version byte    `cbor:"2,keyasint"`
	Length  int64   `cbor:"16,keyasint"`
}

func Send(magic [2]byte, version byte, s io.Writer, obj interface{}) error {
	_obj, err := cbor.Marshal(obj)
	if err != nil {
		return err
	}

	if hdr, err := cbor.Marshal(
		MessageHeader{
			Magic:   magic,
			Version: version,
			Length:  int64(len(_obj)),
		}); err != nil {
		return err
	} else {
		n, err := s.Write([]byte{byte(len(hdr))})
		if err != nil || n != 1 {
			return errors.New("failed to send framer headers length")
		}

		n, err = s.Write(hdr)
		if err != nil || n != len(hdr) {
			return errors.New("failed to send framer headers")
		}
	}

	n, err := s.Write(_obj)
	if err != nil {
		return fmt.Errorf("failed to send framer payload with %w", err)
	}

	if n != len(_obj) {
		return fmt.Errorf("failed to send framer payload in full %d != %d", n, len(_obj))
	}

	return err
}

func Read(magic [2]byte, version byte, s io.Reader, obj interface{}) error {
	hdrLenBuf := []byte{0}
	n, _ := s.Read(hdrLenBuf)
	if n != 1 {
		return errors.New("failed to read headers length")
	}

	var h MessageHeader
	hdrLen := int(hdrLenBuf[0])

	b := make([]byte, hdrLen)
	n, _ = s.Read(b)
	if n != hdrLen {
		return errors.New("failed to read headers")
	}

	if err := cbor.Unmarshal(b, &h); err != nil {
		return err
	}

	if h.Magic[0] != magic[0] || h.Magic[1] != magic[1] {
		return errors.New("unknown protocol")
	}

	if h.Version != version {
		return errors.New("unknown protocol version")
	}

	obj_reader := io.LimitReader(s, int64(h.Length))
	b = make([]byte, h.Length)
	if _, err := io.ReadFull(obj_reader, b); err != nil {
		return err
	}

	return cbor.Unmarshal(b, obj)
}
