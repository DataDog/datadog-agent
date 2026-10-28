package msrpc

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

type pipeShare interface {
	Request() *protocol.Request
}

// Pipe owns a bound RPC named-pipe handle. Callers serialize calls and Close.
type Pipe struct {
	share  pipeShare
	fd     wire.FileId
	callID uint32
}

const pipeCleanupTimeout = 5 * time.Second

// OpenPipe opens a named pipe and binds the requested RPC interface.
func OpenPipe(ctx context.Context, share pipeShare, name string, syntax [16]byte, version uint16) (_ *Pipe, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if share == nil {
		return nil, errInvalidArgument
	}
	callID := rand.Uint32()
	bind := &Bind{CallId: callID, AbstractSyntax: syntax, Version: version}
	res, err := share.Request().WithFollowSymlinks(true).
		Create(name, wire.GENERIC_READ|wire.GENERIC_WRITE, wire.FILE_OPEN, 0, wire.FILE_ATTRIBUTE_NORMAL).
		Ioctl(wire.FSCTL_PIPE_TRANSCEIVE, bind, DefaultMaxFragmentSize).
		Do(ctx)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	createRes, err := res.Create(0)
	if err != nil {
		return nil, err
	}
	p := &Pipe{share: share, fd: createRes.FileId().Decode(), callID: callID}
	defer func() {
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pipeCleanupTimeout)
			defer cancel()
			err = errors.Join(err, p.Close(closeCtx))
		}
	}()

	ioctlRes, err := res.Ioctl(1)
	if err != nil {
		return nil, err
	}
	if err := ValidateBindAck(ioctlRes.Output(), callID); err != nil {
		return nil, err
	}
	return p, nil
}

// Call transceives a request and returns its initial response bytes and call ID.
// The caller reads any remaining fragments before making another call.
func (p *Pipe) Call(ctx context.Context, request func(callID uint32) (wire.Encoder, error)) ([]byte, uint32, error) {
	if ctx == nil {
		panic("nil context")
	}
	if p == nil || p.share == nil || request == nil {
		return nil, 0, errInvalidArgument
	}
	p.callID++
	input, err := request(p.callID)
	if err != nil {
		return nil, p.callID, err
	}
	res, err := p.share.Request().WithFileID(p.fd).
		Ioctl(wire.FSCTL_PIPE_TRANSCEIVE, input, DefaultMaxFragmentSize).Do(ctx)
	if err != nil {
		if data, ok := protocol.BufferOverflowData(err); ok && errors.Is(err, erref.STATUS_BUFFER_OVERFLOW) {
			return data, p.callID, nil
		}
		return nil, p.callID, err
	}
	defer res.Close()
	decoded, err := res.Ioctl(0)
	if err != nil {
		return nil, p.callID, err
	}
	return append([]byte(nil), decoded.Output()...), p.callID, nil
}

// ReadAtLeast reads at least minimum bytes from the pipe into buffer.
func (p *Pipe) ReadAtLeast(ctx context.Context, buffer []byte, minimum int) (int, error) {
	if ctx == nil {
		panic("nil context")
	}
	if p == nil || p.share == nil {
		return 0, errInvalidArgument
	}
	if minimum < 0 || minimum > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	var n int
	for n < minimum {
		res, err := p.share.Request().WithFileID(p.fd).Read(uint32(len(buffer)-n), 0).Do(ctx)
		var data []byte
		if err != nil {
			var ok bool
			data, ok = protocol.BufferOverflowData(err)
			if !ok || len(data) == 0 {
				return n, err
			}
		} else {
			decoded, decodeErr := res.Read(0)
			if decodeErr != nil {
				res.Close()
				return n, decodeErr
			}
			data = decoded.Data()
		}
		if len(data) > len(buffer)-n {
			if res != nil {
				res.Close()
			}
			return n, &InvalidResponseError{Message: "RPC pipe read exceeds buffer"}
		}
		copy(buffer[n:], data)
		if res != nil {
			res.Close()
		}
		if len(data) == 0 {
			return n, io.ErrUnexpectedEOF
		}
		n += len(data)
	}
	return n, nil
}

// Close releases the pipe handle without unmounting its share.
func (p *Pipe) Close(ctx context.Context) error {
	if ctx == nil {
		panic("nil context")
	}
	if p == nil || p.share == nil {
		return nil
	}
	res, err := p.share.Request().WithFileID(p.fd).Close().Do(ctx)
	if err != nil {
		return err
	}
	res.Close()
	return nil
}
