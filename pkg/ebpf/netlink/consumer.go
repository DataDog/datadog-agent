package netlink

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/unix"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

const (
	netlinkCtNew   = uint32(1)
	ipctnlMsgCtGet = 1
	outputBuffer   = 100

	// Minimum BPF sampling rate before we give up
	// See ENOBUF handling below
	minSamplingThreshold = 0.2
)

var msgBufferSize int

func init() {
	msgBufferSize = os.Getpagesize()
}

var errShortErrorMessage = errors.New("not enough data for netlink error code")
var errMaxSamplingAttempts = errors.New("netlink socket creation: too many attempts")

// Consumer is responsible for encapsulating all the logic of hooking into Conntrack
// consuming [NEW] connection events, and exposing a stream of the NAT connections.
type Consumer struct {
	conn         *netlink.Conn
	socket       *Socket
	pool         *bufferPool
	workQueue    chan func()
	samplingRate float64
}

// Event encapsulates the result of a single netlink.Con.Receive() call
type Event struct {
	msgs   []netlink.Message
	err    error
	buffer *[]byte
	pool   *bufferPool
}

// Messages returned from the socket read
func (e *Event) Messages() []netlink.Message {
	return e.msgs
}

// Error associated to the socket read
func (e *Event) Error() error {
	err := e.err
	if err != nil {
		e.Done()
	}
	return err
}

// Done must be called after decoding events so the underlying buffers can be reclaimed.
func (e *Event) Done() {
	if e.buffer != nil {
		e.pool.Put(e.buffer)
	}
}

func NewConsumer(procRoot string) (*Consumer, error) {
	c := &Consumer{
		pool:      newBufferPool(),
		workQueue: make(chan func()),
	}
	c.initWorker(procRoot)

	var err error
	c.do(true, func() {
		samplingRate := 1.0 // Start sampling everything
		err = c.initNetlinkSocket(samplingRate)
	})

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Consumer) Events() <-chan Event {
	output := make(chan Event, outputBuffer)

	c.do(false, func() {
		c.conn.JoinGroup(netlinkCtNew)
		c.receive(output, false)
	})

	return output
}

func (c *Consumer) DumpTable(family uint8) <-chan Event {
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

func (c *Consumer) initNetlinkSocket(samplingRate float64) error {
	var err error

	// This is a safeguard against too many attempts to re-create a socket
	if samplingRate <= samplingThreshold {
		return errMaxSamplingAttempts
	}

	c.socket, err = NewSocket(c.pool)
	if err != nil {
		return err
	}

	if err := setSocketBufferSize(netlinkBufferSize, c.conn); err != nil {
		log.Errorf("error setting rcv buffer size for netlink socket: %s", err)
	}

	if size, err := getSocketBufferSize(c.conn); err == nil {
		log.Debugf("rcv buffer size for netlink socket is %d bytes", size)
	}

	c.conn = netlink.NewConn(c.socket, c.socket.pid)

	// Attach BPF sampling filter if necessary
	c.samplingRate = samplingRate
	if c.samplingRate >= 1.0 {
		return nil
	}

	log.Info("attaching netlink BPF filter with sampling rate: %v", c.samplingRate)
	sampler, _ := GenerateBPFSampler(c.samplingRate)
	err = c.socket.SetBPF(sampler)
	if err != nil {
		return fmt.Errorf("failed to attach BPF filter: %w", err)
	}

	return nil
}

// receive netlink messages and flushes them to the Event channel.
// This method gets called in two different contexts:
//
// - During system-probe startup, when we're loading all entries from the Conntrack table.
// In this casee the `dump` param is set to true, and once we detect the end of the multi-part
// message we stop calling socket.Receive() and close the output channel to signal upstream
// consumers we're done.
//
// - When we're streaming new connection events from the netlink socket. In this case, `dump`
// param is set to false, and only when we detect an EOF we close the output channel.
// It's also worth noting that in the event of an ENOBUF error, we'll re-create a new netlink socket,
// and attach a BPF sampler to it, to lower the the read throughput and save CPU.
func (c *Consumer) receive(output chan Event, dump bool) {
	for {
		c.pool.Reset()
		msgs, err := c.socket.Receive()
		if err != nil {
			switch socketError(err) {
			case errEOF:
				// EOFs are usually indicative of normal program termination, so we simply exit
				return
			case errENOBUF:
				// If we detect an ENOBUF, it means we're not coping with the netlink socket throughput
				// and the receive buffer is overflowing. In that case we throw away the current socket
				// and create a new one with a more aggressive sampling rate.
				log.Warnf("netlink: detected enobuf. will re-create socket with a lower sampling rate.")
				leaveErr := c.conn.LeaveGroup(netlinkCtNew)
				if leaveErr != nil {
					log.Errorf("netlink: error leaving group: %s", leaveErr)
				}

				c.socket.Close()
				err := c.initNetlinkSocket(c.samplingRate / 2)
				if err != nil {
					log.Error("failed re-create netlink socket. exiting conntrack: %s", err)
					return
				}

				// Additionally if the ENOBUF happened during the conntrack dump we just move on as there
				// is no point re-attempting the table dump with a a lower sampling rate
				if dump {
					return
				} else {
					// re-subscribe netlinkCtNew messages
					c.conn.JoinGroup(netlinkCtNew)
					continue
				}
			default:
				// Everything else is propagated upstream
				output <- c.eventFor(nil, err)
			}
		}

		for _, m := range msgs {
			if err := checkMessage(m); err != nil {
				output <- c.eventFor(nil, err)
				return
			}
		}

		// Skip multi-part "done" message
		multiPartDone := len(msgs) > 0 && msgs[len(msgs)-1].Header.Type == netlink.Done
		if multiPartDone {
			msgs = msgs[:len(msgs)-1]
		}

		output <- c.eventFor(msgs, nil)

		// If we're doing a conntrack dump it means we are done after reading the multi-part message
		if dump && multiPartDone {
			return
		}
	}
}

func (c *Consumer) eventFor(msgs []netlink.Message, err error) Event {
	return Event{
		msgs:   msgs,
		err:    err,
		buffer: c.pool.inUse,
		pool:   c.pool,
	}
}

type bufferPool struct {
	inUse *[]byte
	sync.Pool
}

func newBufferPool() *bufferPool {
	return &bufferPool{
		Pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, os.Getpagesize())
				return &b
			},
		},
	}
}

func (b *bufferPool) Get() []byte {
	buf := b.Pool.Get().(*[]byte)
	b.inUse = buf
	return *buf
}

func (b *bufferPool) Reset() {
	b.inUse = nil
}

// Copied from https://github.com/mdlayher/netlink/message.go
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

var (
	errEOF    = errors.New("EOF")
	errENOBUF = errors.New("ENOBUF")
)

// TODO: There is probably a more idiomatic way to do this
func socketError(err error) error {
	if strings.Contains(err.Error(), "closed file") {
		return errEOF
	}

	if strings.Contains(err.Error(), "no buffer space") {
		return errENOBUF
	}

	return err
}
