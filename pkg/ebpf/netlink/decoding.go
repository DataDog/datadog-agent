package netlink

import (
	"log"

	ct "github.com/florianl/go-conntrack"
	"github.com/mdlayher/netlink"
)

func DecodeEvent(logger *log.Logger, e Event) ([]ct.Con, error) {
	// Propagate socket error upstream
	if e.Error != nil {
		return nil, e.Error
	}

	conns := make([]ct.Con, 0, len(e.Reply))
	for _, msg := range e.Reply {
		conn, err := decodeMessage(logger, msg)

		// TODO: handle decoding errors
		if err != nil {
			continue
		}

		conns = append(conns, conn)
	}

	return conns, nil
}

// For now we're using go-conntrack parser, but we should implement our own.
// But we'll soon replace this by a more efficient decoder
func decodeMessage(logger *log.Logger, msg netlink.Message) (ct.Con, error) {
	return ct.ParseAttributes(logger, msg.Data)
}
