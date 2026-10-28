package protocol

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// sendRecvSequential preserves response indexes and handle ownership while
// issuing a related operation group with fewer credits. Unlike a compound,
// each request carries a concrete FileId ([MS-SMB2] 3.2.4.1.4). Stop at the
// first failure; Request closes any handle left open by the group.
func (tc *Tree) sendRecvSequential(ctx context.Context, reqs []wire.Packet) (*Response, error) {
	res := &Response{rpkts: make([]*recvPacket, len(reqs)), tree: tc}
	var fd wire.FileId
	hasFileID := false
	for i, req := range reqs {
		requestedID, usesFileID := requestFileID(req)
		if i == 0 {
			fd = requestedID
			hasFileID = usesFileID
		}
		packet, err := separateFileRequest(req, fd, hasFileID)
		if i > 0 && usesFileID && !hasFileID {
			err = errors.New("protocol: related request has no open file")
		}
		if err == nil {
			var part *Response
			if packet.Command() == wire.SMB2_CREATE {
				part, err = tc.sendRecv(ctx, packet)
			} else {
				var rrs []*outstandingRequest
				rrs, err = tc.send(ctx, packet)
				if err == nil {
					// Retain final responses just as for a CREATE compound.
					// In particular, a canceled CLOSE may still succeed.
					for _, rr := range rrs {
						rr.waitFinal = true
					}
					part, err = recvAll(rrs, tc)
				}
			}
			if err == nil {
				res.rpkts[i] = part.packet(0)
				if packet.Command() == wire.SMB2_CREATE {
					r := wire.CreateResponseDecoder(part.data(0))
					if r.IsInvalid() {
						err = invalidResponse(wire.SMB2_CREATE, "broken create response format")
					} else {
						fd = r.FileId().Decode()
						hasFileID = true
					}
				}
				if packet.Command() != wire.SMB2_CREATE && !usesFileID {
					hasFileID = false
				}
				if ctx.Err() != nil {
					err = ctx.Err()
				}
			}
		}
		if err != nil {
			errs := make([]error, len(reqs))
			for j := i; j < len(errs); j++ {
				errs[j] = err // Failed or not sent because its predecessor failed.
			}
			return res, &CompoundResponseError{Errors: errs}
		}
	}
	return res, nil
}

// Clone before replacing a related FileId: the builder may retry after a
// symlink response or a required-buffer-length error with a different handle.
func separateFileRequest(req wire.Packet, fd wire.FileId, hasFileID bool) (wire.Packet, error) {
	var packet wire.Packet
	var header *wire.PacketHeader
	var fileID *wire.FileId
	switch r := req.(type) {
	case *wire.CreateRequest:
		p := *r
		header = &p.PacketHeader
		packet = &p
	case *wire.CloseRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.FlushRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.QueryInfoRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.SetInfoRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.IoctlRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.QueryDirectoryRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.ReadRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *DirectReadRequest:
		p, read := *r, *r.ReadRequest
		p.ReadRequest = &read
		header = &read.PacketHeader
		packet, fileID = &p, &read.FileId
	case *wire.WriteRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.ChangeNotifyRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	case *wire.LockRequest:
		p := *r
		header = &p.PacketHeader
		packet, fileID = &p, &p.FileId
	default:
		return nil, errors.New("protocol: cannot send this compound command separately")
	}
	if fileID != nil {
		if hasFileID && !fd.IsRelated() {
			*fileID = fd
		} else if fileID.IsRelated() {
			return nil, errors.New("protocol: related request has no open file")
		}
	}
	header.Flags &^= wire.SMB2_FLAGS_RELATED_OPERATIONS
	packet.SetNextCommand(0)
	return packet, nil
}
