package command

import "github.com/taubyte/p2p/streams"

var (
	Magic   = [2]byte{0x01, 0xec}
	Version = byte(0x01)
)

type Body map[string]interface{}

type Command struct {
	conn streams.Connection

	Command string `cbor:"16,keyasint"`
	Body    Body   `cbor:"64,keyasint"`
}
