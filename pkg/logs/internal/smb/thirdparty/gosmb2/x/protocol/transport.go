package protocol

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type directSinkFinder func(head []byte, restSize int) (sink []byte, frontSize int)

// packetStream is the byte stream used by Direct TCP framing. QUIC streams
// implement the same operations, while their connection lifetime is managed
// by quicTransport.
type packetStream interface {
	io.Reader
	io.Writer
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	Close() error
}

// Transport sends and receives complete SMB packets. Close must be safe to
// call concurrently with send and receive operations, must be idempotent, and
// must unblock any operation waiting on the transport.
//
// Transport is a sealed interface implemented by built-in transports
// (such as Direct TCP and SMB over QUIC).
type Transport interface {
	Close() error
	writev(parts ...[]byte) (int, error)
	readPacket(findSink ...directSinkFinder) (*recvPacket, error)
	setReadDeadline(time.Time) error
	setWriteDeadline(time.Time) error
	setPacketReadTimeout(time.Duration)

	// transportType identifies the built-in transport: "tcp" or "quic".
	transportType() string
}

// NewTransport applies framing to conn.
func NewTransport(conn net.Conn) Transport {
	return &transport{conn: conn}
}

type transport struct {
	sb                [4]byte
	conn              packetStream
	packetReadTimeout time.Duration

	recvBuf *recvBuf
	rpos    int
	wpos    int
	// io.Reader permits data and an error together (https://pkg.go.dev/io#Reader).
	// Drain complete buffered frames before reporting this saved error.
	readErr error

	// pending is the number of body bytes of the in-flight packet that have
	// not been consumed by readRestInto yet.
	pending int

	closeOnce sync.Once
	closeErr  error
}

func (t *transport) transportType() string { return "tcp" }

func (t *transport) writev(parts ...[]byte) (n int, err error) {
	size := 0
	for _, p := range parts {
		if len(p) > maxDirectTCPSize-size {
			return -1, errors.New("max transport size exceeds")
		}
		size += len(p)
	}

	be.PutUint32(t.sb[:], uint32(size))

	buffers := append(net.Buffers{t.sb[:]}, parts...)
	n64, err := buffers.WriteTo(t.conn)
	if err != nil {
		return -1, err
	}

	return int(n64), nil
}

func (t *transport) setWriteDeadline(time time.Time) error {
	return t.conn.SetWriteDeadline(time)
}

func (t *transport) setReadDeadline(time time.Time) error {
	return t.conn.SetReadDeadline(time)
}

func (t *transport) setPacketReadTimeout(d time.Duration) {
	t.packetReadTimeout = d
}

func (t *transport) packetReadTimeoutDuration() time.Duration {
	if t.packetReadTimeout > 0 {
		return t.packetReadTimeout
	}
	return clientPacketReadTimeout
}

func (t *transport) dropBuf() {
	if t.recvBuf != nil {
		releaseRecvBuf(t.recvBuf)
		t.recvBuf = nil
	}
	t.rpos = 0
	t.wpos = 0
}

func (t *transport) fill(need int) error {
	if t.wpos-t.rpos >= need {
		return nil
	}
	if t.readErr != nil {
		t.dropBuf()
		return t.readErr
	}

	if t.recvBuf == nil {
		t.recvBuf = allocRecvBuf(need)
		t.rpos = 0
		t.wpos = 0
	} else if t.rpos+need > len(t.recvBuf.data) {
		avail := t.wpos - t.rpos
		newBuf := allocRecvBuf(avail + need)
		if avail > 0 {
			copy(newBuf.data[:avail], t.recvBuf.data[t.rpos:t.wpos])
		}
		releaseRecvBuf(t.recvBuf)
		t.recvBuf = newBuf
		t.rpos = 0
		t.wpos = avail
	}

	for t.wpos-t.rpos < need {
		n, err := t.conn.Read(t.recvBuf.data[t.wpos:])
		t.wpos += n
		if err != nil {
			t.readErr = err
		}
		if t.wpos-t.rpos >= need {
			return nil
		}
		if t.readErr != nil {
			t.dropBuf()
			return t.readErr
		}
	}

	return nil
}

// readRestInto consumes len(b) bytes of the in-flight packet body, first from
// the internal buffer, then directly from the underlying connection, writing
// them into b without an intermediate copy.
func (t *transport) readRestInto(b []byte) error {
	if len(b) > t.pending {
		t.dropBuf()
		return errors.New("incomplete packet")
	}
	t.pending -= len(b)

	// consume bytes already buffered by previous reads
	if t.recvBuf != nil {
		if n := copy(b, t.recvBuf.data[t.rpos:t.wpos]); n > 0 {
			t.rpos += n
			b = b[n:]
		}
	}

	if len(b) > 0 {
		if t.readErr != nil {
			t.dropBuf()
			return t.readErr
		}

		for len(b) > 0 {
			n, err := t.conn.Read(b)
			if n > 0 {
				b = b[n:]
			}
			if err != nil {
				t.readErr = err
			}
			if len(b) == 0 {
				break
			}
			if t.readErr != nil {
				t.dropBuf()
				return t.readErr
			}
		}
	}

	if t.pending == 0 && t.rpos == t.wpos {
		t.dropBuf()
	}

	return nil
}

func (t *transport) readPacket(findSink ...directSinkFinder) (*recvPacket, error) {
	if err := t.fill(4); err != nil {
		return nil, err
	}

	header := t.recvBuf.data[t.rpos : t.rpos+4]
	if header[0] != 0 {
		t.dropBuf()
		return nil, errors.New("invalid transport format")
	}

	pktSize := int(be.Uint32(header))
	if pktSize > maxDirectTCPSize {
		t.dropBuf()
		return nil, errors.New("max transport size exceeds")
	}
	t.rpos += 4

	if t.wpos-t.rpos < pktSize {
		if err := t.conn.SetReadDeadline(time.Now().Add(t.packetReadTimeoutDuration())); err != nil {
			t.dropBuf()
			return nil, err
		}
		defer t.conn.SetReadDeadline(time.Time{})
	}

	n := min(pktSize, 80)
	if err := t.fill(n); err != nil {
		return nil, err
	}

	t.pending = pktSize - n
	head := t.recvBuf.data[t.rpos : t.rpos+n]
	t.rpos += n

	if len(findSink) > 0 && findSink[0] != nil {
		if sink, frontSize := findSink[0](head, t.pending); sink != nil {
			if frontSize < len(head) || frontSize > pktSize || len(sink) != pktSize-frontSize {
				t.dropBuf()
				return nil, errors.New("invalid direct sink size")
			}
			rp := allocRecvPacket(frontSize)
			copy(rp.pkt[:len(head)], head)
			if pad := frontSize - len(head); pad > 0 {
				if err := t.readRestInto(rp.pkt[len(head):frontSize]); err != nil {
					rp.close()
					return nil, err
				}
			}
			if len(sink) > 0 {
				if err := t.readRestInto(sink); err != nil {
					rp.close()
					return nil, err
				}
			}
			rp.ext = sink
			return rp, nil
		}
	}

	spare := 0
	if len(head) >= 4 && head[0] == 0xfd && head[1] == 'S' && head[2] == 'M' && head[3] == 'B' {
		// [MS-SMB2] 2.2.41 stores the 16-byte authentication tag in the
		// transform header. Spare tail capacity lets session.decrypt append it
		// to the ciphertext and authenticate/decrypt in the receive buffer.
		spare = 16
	}
	rp := allocRecvPacketWithSpare(pktSize, spare)
	copy(rp.pkt[:len(head)], head)
	if t.pending > 0 {
		if err := t.readRestInto(rp.pkt[len(head):]); err != nil {
			rp.close()
			return nil, err
		}
	}

	return rp, nil
}

func (t *transport) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.conn.Close()
	})
	return t.closeErr
}

var _ Transport = (*transport)(nil)
