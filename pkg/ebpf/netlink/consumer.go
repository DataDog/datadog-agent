package netlink

import (
	stdlog "log"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/mdlayher/netlink"
)

const netlinkCtNew = uint32(1)

// Consumer is responsible for encapsulating all the logic of hooking into Conntrack
// consuming [NEW] connection events, and exposing a stream of the NAT connections.
type Consumer struct {
	conn      *netlink.Conn
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
		socket, err := NewSocket(c.pool)
		if err != nil {
			return
		}

		c.conn = netlink.NewConn(socket, socket.pid)

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
	output := make(chan Event, 5)

	c.do(false, func() {
		defer close(output)
		c.conn.JoinGroup(netlinkCtNew)
		for {
			c.pool.Reset()
			reply, err := c.conn.Receive()
			e := Event{Reply: reply, Error: err, buffers: c.pool.inUse, pool: c.pool}
			output <- e

			// TODO: Is it enough to propagate the error upstream?
			if err != nil {
				return
			}
		}
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
