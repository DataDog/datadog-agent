package netlink

import (
	"errors"
	stdlog "log"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/unix"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

const netlinkCtNew = uint32(1)
const ipctnlMsgCtGet = 1
const outputBuffer = 100

var errShortErrorMessage = errors.New("not enough data for netlink error code")

// Consumer is responsible for encapsulating all the logic of hooking into Conntrack
// consuming [NEW] connection events, and exposing a stream of the NAT connections.
type Consumer struct {
	conn      *netlink.Conn
	socket    *Socket
	pool      *bufferPool
	logger    *stdlog.Logger
	workQueue chan func()
}

// Event encapsulates the result of a single netlink.Con.Receive() call
type Event struct {
	Reply []netlink.Message
	Error error

	buffers [][]byte
	pool    *bufferPool
}

// Done must be called after decoding events so the underlying buffers can be reclaimed.
func (e *Event) Done() {
	for _, b := range e.buffers {
		e.pool.Put(b)
	}
}

func NewConsumer(procRoot string, logger *stdlog.Logger) (*Consumer, error) {
	c := &Consumer{
		pool:      newBufferPool(),
		logger:    logger,
		workQueue: make(chan func()),
	}
	c.initWorker(procRoot)

	// Create netlink socket within root namespace
	var err error
	c.do(true, func() {
		c.socket, err = NewSocket(c.pool)
		if err != nil {
			return
		}

		c.conn = netlink.NewConn(c.socket, c.socket.pid)

		// Sometimes the conntrack flushes are larger than the socket recv buffer capacity.
		// This ensures that in case of buffer overrun the `recvmsg` call will *not*
		// receive an ENOBUF which is currently not handled properly by go-conntrack library.
		c.conn.SetOption(netlink.NoENOBUFS, true)

		// We also increase the socket buffer size to better handle bursts of events.
		if err := setSocketBufferSize(netlinkBufferSize, c.conn); err != nil {
			log.Errorf("error setting rcv buffer size for netlink socket: %s", err)
		}

		if size, err := getSocketBufferSize(c.conn); err == nil {
			log.Debugf("rcv buffer size for netlink socket is %d bytes", size)
		}
	})

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Consumer) Events() chan Event {
	output := make(chan Event, outputBuffer)

	c.do(false, func() {
		c.conn.JoinGroup(netlinkCtNew)
		c.receive(output, false)
	})

	return output
}

func (c *Consumer) DumpTable(family uint8) chan Event {
	output := make(chan Event, outputBuffer)

	c.do(false, func() {
		defer close(output)

		req := netlink.Message{
			Header: netlink.Header{
				Flags: netlink.Request | netlink.Dump,
				Type:  netlink.HeaderType((unix.NFNL_SUBSYS_CTNETLINK << 8) | ipctnlMsgCtGet),
			},
			Data: []byte{family, unix.NFNETLINK_V0, 0, 0},
		}

		verify, err := c.conn.Send(req)
		if err != nil {
			output <- c.eventFor(nil, err)
			return
		}

		if err := netlink.Validate(req, []netlink.Message{verify}); err != nil {
			output <- c.eventFor(nil, err)
			return
		}

		c.receive(output, true)
	})

	return output
}

func (c *Consumer) Stop() {
	c.conn.Close()
}

func (c *Consumer) initWorker(procRoot string) {
	go func() {
		util.WithRootNS(procRoot, func() {
			for {
				fn, ok := <-c.workQueue
				if !ok {
					return
				}
				fn()
			}
		})
	}()
}

func (c *Consumer) do(sync bool, fn func()) {
	if !sync {
		c.workQueue <- fn
		return
	}

	done := make(chan struct{})
	syncFn := func() {
		fn()
		close(done)
	}
	c.workQueue <- syncFn
	<-done
}

// based on of https://github.com/mdlayher/netlink/conn.go
// however in our context we don't care about multi-part messages in the sense
// that is better to flush their parts the output channel and create back-pressure
// instead of buffering them in memory. when dump is set to true we terminate
// after processing the first multi-part message.
func (c *Consumer) receive(output chan Event, dump bool) {
	for {
		c.pool.Reset()
		msgs, err := c.socket.Receive()
		if err != nil {
			if !isEOF(err) {
				output <- c.eventFor(nil, err)
			}
			return
		}

		for _, m := range msgs {
			if err := checkMessage(m); err != nil {
				output <- c.eventFor(nil, err)
				return
			}
		}

		multiPartDone := len(msgs) > 0 && msgs[len(msgs)-1].Header.Type == netlink.Done
		if multiPartDone {
			msgs = msgs[:len(msgs)-1]
		}

		output <- c.eventFor(msgs, nil)

		// if we're doing a conntrack dump it means we are done after reading the multi-part message
		if dump && multiPartDone {
			return
		}
	}
}

func (c *Consumer) eventFor(msgs []netlink.Message, err error) Event {
	return Event{
		Reply:   msgs,
		Error:   err,
		buffers: c.pool.inUse,
		pool:    c.pool,
	}
}

type bufferPool struct {
	inUse [][]byte
	sync.Pool
}

func newBufferPool() *bufferPool {
	return &bufferPool{
		Pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, os.Getpagesize())
			},
		},
	}
}

func (b *bufferPool) Get() []byte {
	buf := b.Pool.Get().([]byte)
	b.inUse = append(b.inUse, buf)
	return buf
}

func (b *bufferPool) Reset() {
	b.inUse = nil
}

// source: https://github.com/mdlayher/netlink/message.go
// checkMessage checks a single Message for netlink errors.
func checkMessage(m netlink.Message) error {
	const success = 0

	// Per libnl documentation, only messages that indicate type error can
	// contain error codes:
	// https://www.infradead.org/~tgr/libnl/doc/core.html#core_errmsg.
	//
	// However, at one point, this package checked both done and error for
	// error codes.  Because there was no issue associated with the change,
	// it is unknown whether this change was correct or not.  If you run into
	// a problem with your application because of this change, please file
	// an issue.
	if m.Header.Type != netlink.Error {
		return nil
	}

	if len(m.Data) < 4 {
		return errShortErrorMessage
	}

	if c := nlenc.Int32(m.Data[0:4]); c != success {
		// Error code is a negative integer, convert it into an OS-specific raw
		// system call error, but do not wrap with os.NewSyscallError to signify
		// that this error was produced by a netlink message; not a system call.
		return syscall.Errno(-1 * int(c))
	}

	return nil
}

// TODO: Validate if there is a better way to check for EOF
func isEOF(err error) bool {
	return strings.Contains(err.Error(), "closed file")
}
