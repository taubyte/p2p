package packer

import (
	"encoding/binary"
	"errors"
	"io"
)

type packer struct {
	magic   Magic
	version Version
}

type Magic [2]byte
type Channel uint8
type Version uint16

type Packer interface {
	Send(Channel, io.Writer, io.Reader, int64) error
	Recv(io.Reader, io.Writer) (Channel, int64, error)
}

func New(magic Magic, version Version) Packer {
	p := packer{
		version: version,
	}
	p.magic[0] = magic[0]
	p.magic[1] = magic[1]
	return p
}

func (p packer) Send(channel Channel, w io.Writer, r io.Reader, length int64) error {
	_, err := w.Write(p.magic[:])
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, p.version)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, length)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.LittleEndian, channel)
	if err != nil {
		return err
	}

	lr := io.LimitReader(r, length)

	n, err := io.Copy(w, lr)
	if n != length {
		return io.ErrShortWrite
	}

	if err != nil {
		return err
	}

	return nil
}

func (p packer) Recv(r io.Reader, w io.Writer) (Channel, int64, error) {
	_magic := make([]byte, 2)
	_, err := r.Read(_magic)
	if err != nil {
		return 0, 0, err
	}

	if _magic[0] != p.magic[0] || _magic[1] != p.magic[1] {
		return 0, 0, errors.New("wrong packer magic")
	}

	var version Version
	err = binary.Read(r, binary.LittleEndian, &version)
	if err != nil {
		return 0, 0, err
	}

	if version != p.version {
		return 0, 0, errors.New("wrong packer version")
	}

	var length int64
	err = binary.Read(r, binary.LittleEndian, &length)
	if err != nil {
		return 0, 0, err
	}

	var channel Channel
	err = binary.Read(r, binary.LittleEndian, &channel)
	if err != nil {
		return 0, 0, err
	}

	lr := io.LimitReader(r, length)

	n, err := io.Copy(w, lr)

	return channel, n, err
}
