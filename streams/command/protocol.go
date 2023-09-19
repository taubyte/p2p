package command

import (
	"github.com/taubyte/p2p/streams"
	"github.com/taubyte/p2p/streams/packer"
)

var (
	Magic   = packer.Magic{0x01, 0xec}
	Version = packer.Version(0x01)
)

type Body map[string]interface{}

type Command struct {
	conn streams.Connection

	Command string `cbor:"16,keyasint"`
	Body    Body   `cbor:"64,keyasint"`
}
