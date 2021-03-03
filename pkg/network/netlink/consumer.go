// +build linux
// +build !android

package netlink

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"github.com/pkg/errors"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

const (
	// netlinkCtNew represents the Netlink multicast group associated to the Conntrack family
	// representing new connection events. For more information see section "Address formats" in
	// http://man7.org/linux/man-pages/man7/netlink.7.html
	netlinkCtNew = uint32(1)

	// ipctnlMsgCtGet represents the Conntrack message type used during the initial load.
	// This value is defined in include/uapi/linux/netfilter/nfnetlink_conntrack.h
	ipctnlMsgCtGet = 1

	// outputBuffer is he size of the Consumer output channel.
	outputBuffer = 100

	// overShootFactor is used sampling rate calculation after the circuit breaker trips.
	overshootFactor = 0.95

	// netlinkBufferSize is size (in bytes) of the Netlink socket receive buffer
	// We set it to a large enough size to support bursts of Conntrack events.
	netlinkBufferSize = 1024 * 1024
)

var errShortErrorMessage = errors.New("not enough data for netlink error code")
var pre315Kernel bool

func init() {
	if vers, err := kernel.HostVersion(); err == nil {
		pre315Kernel = vers < kernel.VersionCode(3, 15, 0)
	}
}

// Consumer is responsible for encapsulating all the logic of hooking into Conntrack via a Netlink socket
// and streaming new connection events.
type Consumer struct {
	conn      *netlink.Conn
	socket    *Socket
	pool      *sync.Pool
	workQueue chan func()
	procRoot  string

	// targetRateLimit represents the maximum number of netlink messages per second
	// that can be read off the netlink socket. Setting it to -1 disables the limit.
	targetRateLimit int

	// samplingRate must be a value between 0 and 1 (inclusive) which is adjusted dynamically.
	// this represents the amount of sampling we apply to the netlink socket via a BPF filter
	// to reach the targetRateLimit.
	samplingRate float64

	// breaker is meant to ensure we never process more netlink messages than the specified targetRateLimit.
	// when the circuit breaker trips, we close the socket and re-create a new one with the samplingRate
	// adjusted accordingly to meet the desired targetRateLimit.
	breaker *CircuitBreaker

	// streaming is set to true after we finish the initial Conntrack dump.
	streaming bool

	// telemetry
	enobufs     int64
	throttles   int64
	samplingPct int64
	readErrors  int64
	msgErrors   int64

	netlinkSeqNumber    uint32
	listenAllNamespaces bool

	// for testing purposes
	recvLoopRunning int32
}

// Event encapsulates the result of a single netlink.Con.Receive() call
type Event struct {
	msgs   []netlink.Message
	netns  int32
	buffer *[]byte
	pool   *sync.Pool
}

// Messages returned from the socket read
func (e *Event) Messages() []netlink.Message {
	return e.msgs
}

// Done must be called after decoding events so the underlying buffers can be reclaimed.
func (e *Event) Done() {
	if e.buffer != nil {
		e.pool.Put(e.buffer)
	}
}

// NewConsumer creates a new Conntrack event consumer.
// targetRateLimit represents the maximum number of netlink messages per second that can be read off the socket
func NewConsumer(procRoot string, targetRateLimit int, listenAllNamespaces bool) *Consumer {
	c := &Consumer{
		procRoot:            procRoot,
		pool:                newBufferPool(),
		workQueue:           make(chan func()),
		targetRateLimit:     targetRateLimit,
		breaker:             NewCircuitBreaker(int64(targetRateLimit)),
		netlinkSeqNumber:    1,
		listenAllNamespaces: listenAllNamespaces,
	}

	c.initWorker(procRoot)

	return c
}

// Events returns a channel of Event objects (wrapping netlink messages) which receives
// all new connections added to the Conntrack table.
func (c *Consumer) Events() (<-chan Event, error) {
	if err := c.initNetlinkSocket(1.0); err != nil {
		return nil, fmt.Errorf("could not initialize conntrack netlink socket: %w", err)
	}

	output := make(chan Event, outputBuffer)

	c.do(false, func() {
		defer func() {
			log.Info("exited conntrack netlink receive loop")
			close(output)
		}()

		c.streaming = true
		_ = c.conn.JoinGroup(netlinkCtNew)
		c.receive(output)
	})

	return output, nil
}

// isPeerNS determines whether the given network namespace is a peer
// of the given netlink socket
func (c *Consumer) isPeerNS(conn *netlink.Conn, ns netns.NsHandle) bool {
	encoder := netlink.NewAttributeEncoder()
	encoder.Uint32(unix.NETNSA_FD, uint32(ns))
	data, err := encoder.Encode()
	if err != nil {
		log.Errorf("isPeerNS: err encoding attributes netlink attributes: %s", err)
		return false
	}

	msg := netlink.Message{
		Header: netlink.Header{
			Flags:    netlink.Request,
			Type:     unix.RTM_GETNSID,
			Sequence: c.netlinkSeqNumber,
		},
		Data: []byte{unix.AF_UNSPEC, 0, 0, 0},
	}

	msg.Data = append(msg.Data, data...)

	if msg, err = conn.Send(msg); err != nil {
		log.Errorf("isPeerNS: err sending netlink request: %s", err)
		return false
	}

	msgs, err := conn.Receive()
	if err != nil {
		log.Errorf("isPeerNS: error receiving netlink reply: %s", err)
		return false
	}

	log.Tracef("netlink reply: %v", msgs)

	if msgs[0].Header.Type == netlink.Error {
		return false
	}

	c.netlinkSeqNumber++

	decoder, err := netlink.NewAttributeDecoder(msgs[0].Data)
	if err != nil {
		return false
	}

	for {
		if decoder.Type() == unix.NETNSA_NSID {
			return int32(decoder.Uint32()) >= 0
		}
		if !decoder.Next() {
			break
		}
	}

	return false
}

// DumpTable returns a channel of Event objects containing all entries
// present in the Conntrack table. The channel is closed once all entries are read.
// This method is meant to be used once during the process initialization of system-probe.
func (c *Consumer) DumpTable(family uint8) (<-chan Event, error) {
	var nss []netns.NsHandle
	var err error
	if c.listenAllNamespaces {
		nss, err = util.GetNetNamespaces(c.procRoot)
		if err != nil {
			return nil, fmt.Errorf("error dumping conntrack table, could not get network namespaces: %w", err)
		}
	}

	rootNS, err := util.GetRootNetNamespace(c.procRoot)
	if err != nil {
		return nil, fmt.Errorf("error dumping conntrack table, could not get root namespace: %w", err)
	}

	conn, err := netlink.Dial(unix.AF_UNSPEC, &netlink.Config{NetNS: int(rootNS)})
	if err != nil {
		rootNS.Close()
		return nil, fmt.Errorf("error dumping conntrack table, could not open netlink socket: %w", err)
	}

	output := make(chan Event, outputBuffer)

	go func() {
		defer func() {
			for _, ns := range nss {
				_ = ns.Close()
			}

			close(output)

			_ = rootNS.Close()
			_ = conn.Close()
		}()

		// root ns first
		if err := c.dumpTable(family, output, rootNS); err != nil {
			log.Errorf("error dumping conntrack table for root namespace, some NAT info may be missing: %s", err)
		}

		for _, ns := range nss {
			if rootNS.Equal(ns) {
				// we've already dumped the table for the root ns above
				continue
			}

			if !c.isPeerNS(conn, ns) {
				log.Tracef("not dumping ns %s since it is not a peer of the root ns", ns)
				continue
			}

			if err := c.dumpTable(family, output, ns); err != nil {
				log.Errorf("error dumping conntrack table for namespace %d: %s", ns, err)
			}
		}
	}()

	return output, nil
}

func (c *Consumer) dumpTable(family uint8, output chan Event, ns netns.NsHandle) error {
	return util.WithNS(c.procRoot, ns, func() error {

		log.Tracef("dumping table for ns %s family %d", ns, family)

		sock, err := NewSocket()
		if err != nil {
			return fmt.Errorf("could not open netlink socket for net ns %d: %w", int(ns), err)
		}

		conn := netlink.NewConn(sock, sock.pid)

		defer func() {
			_ = conn.Close()
		}()

		req := netlink.Message{
			Header: netlink.Header{
				Type:  netlink.HeaderType((unix.NFNL_SUBSYS_CTNETLINK << 8) | ipctnlMsgCtGet),
				Flags: netlink.Request | netlink.Dump,
			},
			Data: []byte{family, unix.NFNETLINK_V0, 0, 0},
		}

		verify, err := conn.Send(req)
		if err != nil {
			return fmt.Errorf("netlink dump error: %w", err)
		}

		if err := netlink.Validate(req, []netlink.Message{verify}); err != nil {
			return fmt.Errorf("netlink dump message validation error: %w", err)
		}

		c.socket = sock
		c.receive(output)
		return nil
	})
}

// GetStats returns telemetry associated to the Consumer
func (c *Consumer) GetStats() map[string]int64 {
	return map[string]int64{
		"enobufs":      atomic.LoadInt64(&c.enobufs),
		"throttles":    atomic.LoadInt64(&c.throttles),
		"sampling_pct": atomic.LoadInt64(&c.samplingPct),
		"read_errors":  atomic.LoadInt64(&c.readErrors),
		"msg_errors":   atomic.LoadInt64(&c.msgErrors),
	}
}

// Stop the consumer
func (c *Consumer) Stop() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// initWorker creates a go-routine *within the root network namespace*.
// This go-routine is responsible for all socket system calls.
func (c *Consumer) initWorker(procRoot string) {
	go func() {
		_ = util.WithRootNS(procRoot, func() error {
			for {
				fn, ok := <-c.workQueue
				if !ok {
					return nil
				}
				fn()
			}
		})
	}()
}

// do simply dispatches an action to the go-routine running within the root network
// namespace. the caller can wait for the execution to finish by setting sync to true.
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
	c.socket, err = NewSocket()
	if err != nil {
		return err
	}

	c.conn = netlink.NewConn(c.socket, c.socket.pid)

	// We use this as opposed to netlink.Conn.SetReadBuffer because you can only
	// set a value higher than /proc/sys/net/core/rmem_default (which is around 200kb for most systems)
	// if you use SO_RCVBUFFORCE with CAP_NET_ADMIN (https://linux.die.net/man/7/socket).
	if err := c.socket.SetSockoptInt(syscall.SOL_SOCKET, syscall.SO_RCVBUFFORCE, netlinkBufferSize); err != nil {
		log.Errorf("error setting rcv buffer size for netlink socket: %s", err)
	}

	if size, err := c.socket.GetSockoptInt(syscall.SOL_SOCKET, syscall.SO_RCVBUF); err == nil {
		log.Debugf("rcv buffer size for netlink socket is %d bytes", size)
	}

	if c.listenAllNamespaces {
		if err := c.socket.SetSockoptInt(unix.SOL_NETLINK, unix.NETLINK_LISTEN_ALL_NSID, 1); err != nil {
			log.Errorf("error enabling listen for all namespaces on netlink socket: %s", err)
		}
	}

	// Attach BPF sampling filter if necessary
	c.samplingRate = samplingRate
	atomic.StoreInt64(&c.samplingPct, int64(samplingRate*100.0))
	if c.samplingRate >= 1.0 {
		return nil
	}

	log.Infof("attaching netlink BPF filter with sampling rate: %.2f", c.samplingRate)
	sampler, _ := GenerateBPFSampler(c.samplingRate)
	err = c.socket.SetBPF(sampler)
	if err != nil {
		atomic.StoreInt64(&c.samplingPct, 0)
		return fmt.Errorf("failed to attach BPF filter: %w", err)
	}

	return nil
}

// receive netlink messages and flushes them to the Event channel.
// This method gets called in two different contexts:
//
// - During system-probe startup, when we're loading all entries from the Conntrack table.
// In this case c.streaming attribute is false, and once we detect the end of the multi-part
// message we stop calling socket.Receive() and close the output channel to signal upstream
// consumers we're done.
//
// - When we're streaming new connection events from the netlink socket. In this case, `c.streaming`
// attribute is true, and only when we detect an EOF we close the output channel.
// It's also worth noting that in the event of an ENOBUF error, we'll re-create a new netlink socket,
// and attach a BPF sampler to it, to lower the the read throughput and save CPU.
func (c *Consumer) receive(output chan Event) {
	atomic.StoreInt32(&c.recvLoopRunning, 1)
	defer func() {
		atomic.StoreInt32(&c.recvLoopRunning, 0)
	}()

ReadLoop:
	for {
		buffer := c.pool.Get().(*[]byte)
		msgs, netns, err := c.socket.ReceiveInto(*buffer)

		if err != nil {
			log.Tracef("consumer netlink socket error: %s", err)
			switch socketError(err) {
			case errEOF:
				// EOFs are usually indicative of normal program termination, so we simply exit
				return
			case errENOBUF:
				atomic.AddInt64(&c.enobufs, 1)
			default:
				atomic.AddInt64(&c.readErrors, 1)
			}
		}

		if err := c.throttle(len(msgs)); err != nil {
			log.Errorf("exiting conntrack netlink consumer loop due to throttling error: %s", err)
			return
		}

		// Messages with error codes are simply skipped
		for _, m := range msgs {
			if err := checkMessage(m); err != nil {
				atomic.AddInt64(&c.msgErrors, 1)
				continue ReadLoop
			}
		}

		// Skip multi-part "done" messages
		multiPartDone := len(msgs) > 0 && msgs[len(msgs)-1].Header.Type == netlink.Done
		if multiPartDone {
			msgs = msgs[:len(msgs)-1]
		}

		output <- c.eventFor(msgs, netns, buffer)

		// If we're doing a conntrack dump we terminate after reading the multi-part message
		if multiPartDone && !c.streaming {
			return
		}
	}
}

func (c *Consumer) eventFor(msgs []netlink.Message, netns int32, buffer *[]byte) Event {
	return Event{
		msgs:   msgs,
		netns:  netns,
		buffer: buffer,
		pool:   c.pool,
	}
}

// throttle ensures that the read throughput from the socket stays below
// the configured maxMessagePerSecond
func (c *Consumer) throttle(numMessages int) error {
	// We don't throttle the socket during initialization
	// (when we dump the whole Conntrack table)
	if !c.streaming {
		return nil
	}

	c.breaker.Tick(numMessages)
	if !c.breaker.IsOpen() {
		return nil
	}
	atomic.AddInt64(&c.throttles, 1)

	// Close current socket
	c.conn.Close()
	c.conn = nil

	if pre315Kernel {
		// we cannot recreate the socket and set a bpf filter on
		// kernels before 3.15, so we bail here
		log.Errorf("conntrack sampling not supported on kernel versions < 3.15. Please adjust system_probe_config.conntrack_rate_limit (currently set to %d) to accommodate higher conntrack update rate detected", c.targetRateLimit)
		return fmt.Errorf("conntrack sampling rate not supported")
	}

	// Create new socket with the desired sampling rate
	// We calculate the required sampling rate to reach the target maxMessagesPersecond
	samplingRate := (float64(c.targetRateLimit) / float64(c.breaker.Rate())) * c.samplingRate * overshootFactor
	err := c.initNetlinkSocket(samplingRate)
	if err != nil {
		log.Errorf("failed to re-create netlink socket. exiting conntrack: %s", err)
		return err
	}

	// Reset circuit breaker
	c.breaker.Reset()
	// Re-subscribe netlinkCtNew messages
	return c.conn.JoinGroup(netlinkCtNew)
}

func newBufferPool() *sync.Pool {
	bufferSize := os.Getpagesize()
	return &sync.Pool{
		New: func() interface{} {
			b := make([]byte, bufferSize)
			return &b
		},
	}
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
