package framer

import (
	"encoding/gob"
	"errors"
	"io"

	"github.com/fxamacker/cbor/v2"
)

type MessageHeader struct {
	Magic   [2]byte
	Version byte
	Length  int
}

func Send(magic [2]byte, version byte, s io.Writer, obj interface{}) error {
	_obj, err := cbor.Marshal(obj)
	if err != nil {
		return err
	}

	if err = gob.NewEncoder(s).Encode(
		MessageHeader{
			Magic:   magic,
			Version: version,
			Length:  len(_obj),
		}); err != nil {
		return err
	}

	// TODO: make sure we're sending all
	_, err = s.Write(_obj)
	return err
}

func Read(magic [2]byte, version byte, s io.Reader, obj interface{}) error {
	var h MessageHeader

	if err := gob.NewDecoder(s).Decode(&h); err != nil {
		return err
	}

	if h.Magic[0] != magic[0] || h.Magic[1] != magic[1] {
		return errors.New("unknown protocol")
	}

	if h.Version != version {
		return errors.New("unknown protocol version")
	}

	obj_reader := io.LimitReader(s, int64(h.Length))
	b := make([]byte, h.Length)
	if _, err := io.ReadFull(obj_reader, b); err != nil {
		return err
	}

	return cbor.Unmarshal(b, obj)
}
