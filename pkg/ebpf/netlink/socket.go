// +build linux

package netlink

import (
	"errors"
	"math"
	"os"
	"syscall"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

var _ netlink.Socket = &Socket{}
var errNotImplemented = errors.New("not implemented")

// Socket is pretty much a copy of netlink.conn without the unnecessary cruft.
// We introduce a `ReceiveInto` method aimed at reducing memory allocations
// and remove all synchronization points and go-routines from it.
type Socket struct {
	fd   *os.File
	pid  uint32
	conn syscall.RawConn
	pool *bufferPool
}

func NewSocket(pool *bufferPool) (*Socket, error) {
	fd, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW|unix.SOCK_CLOEXEC,
		unix.NETLINK_NETFILTER,
	)

	if err != nil {
		return nil, err
	}

	err = unix.SetNonblock(fd, true)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}

	err = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK})
	if err != nil {
		syscall.Close(fd)
		return nil, os.NewSyscallError("bind", err)
	}

	addr, err := unix.Getsockname(fd)
	if err != nil {
		syscall.Close(fd)
		return nil, os.NewSyscallError("getsockname", err)
	}

	pid := addr.(*unix.SockaddrNetlink).Pid
	file := os.NewFile(uintptr(fd), "netlink")

	conn, err := file.SyscallConn()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &Socket{fd: file, pid: pid, conn: conn, pool: pool}, nil
}

func (s *Socket) Send(m netlink.Message) error {
	b, err := m.MarshalBinary()
	if err != nil {
		return err
	}

	addr := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
	}

	ctrlErr := s.conn.Write(func(fd uintptr) bool {
		err = unix.Sendmsg(int(fd), b, nil, addr, 0)
		return ready(err)
	})
	if ctrlErr != nil {
		return ctrlErr
	}

	return err
}

func (s *Socket) Receive() ([]netlink.Message, error) {
	b := s.pool.Get()
	return s.ReceiveInto(b)
}

// ReceiveInto takes in buffer and returns the netlink messages in the socket.
// Notice the use of the pre-allocated buffer is just a best effort, as the message might not
// fit in it. In that case we allocate a new buffer doubling it's capacity.
func (s *Socket) ReceiveInto(b []byte) ([]netlink.Message, error) {
	for {
		// Peek at the buffer to see how many bytes are available.
		n, err := s.recvmsg(b, unix.MSG_PEEK)
		if err != nil {
			return nil, os.NewSyscallError("recvmsg", err)
		}

		// Break when we can read all messages
		if n < len(b) {
			break
		}

		// Double in size if not enough bytes
		b = make([]byte, len(b)*2)
	}

	// Read out all available messages
	n, err := s.recvmsg(b, 0)
	if err != nil {
		return nil, os.NewSyscallError("recvmsg", err)
	}

	n = nlmsgAlign(n)

	raw, err := syscall.ParseNetlinkMessage(b[:n])
	if err != nil {
		return nil, err
	}

	msgs := make([]netlink.Message, 0, len(raw))
	for _, r := range raw {
		m := netlink.Message{
			Header: sysToHeader(r.Header),
			Data:   r.Data,
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (s *Socket) File() *os.File {
	return s.fd
}

func (s *Socket) Close() error {
	return s.fd.Close()
}

func (s *Socket) SendMessages(m []netlink.Message) error {
	return errNotImplemented
}

func (s *Socket) JoinGroup(group uint32) error {
	return os.NewSyscallError("setsockopt", s.setSockoptInt(
		unix.SOL_NETLINK,
		unix.NETLINK_ADD_MEMBERSHIP,
		int(group),
	))
}

func (s *Socket) LeaveGroup(group uint32) error {
	return os.NewSyscallError("setsockopt", s.setSockoptInt(
		unix.SOL_NETLINK,
		unix.NETLINK_DROP_MEMBERSHIP,
		int(group),
	))
}

func (s *Socket) setSockoptInt(level, opt, value int) error {
	// Value must be in range of a C integer.
	if value < math.MinInt32 || value > math.MaxInt32 {
		return unix.EINVAL
	}

	var err error
	ctrlErr := s.conn.Control(func(fd uintptr) {
		err = unix.SetsockoptInt(int(fd), level, opt, value)
	})
	if ctrlErr != nil {
		return ctrlErr
	}

	return err
}

func (s *Socket) recvmsg(b []byte, flags int) (int, error) {
	var (
		n   int
		err error
	)

	ctrlErr := s.conn.Read(func(fd uintptr) bool {
		n, _, _, _, err = unix.Recvmsg(int(fd), b, nil, flags)
		return ready(err)
	})

	if ctrlErr != nil {
		return 0, ctrlErr
	}

	return n, err
}

// ready indicates readiness based on the value of err.
func ready(err error) bool {
	// When a socket is in non-blocking mode, we might see
	// EAGAIN. In that case, return false to let the poller wait for readiness.
	// See the source code for internal/poll.FD.RawRead for more details.
	//
	// Starting in Go 1.14, goroutines are asynchronously preemptible. The 1.14
	// release notes indicate that applications should expect to see EINTR more
	// often on slow system calls (like recvmsg while waiting for input), so
	// we must handle that case as well.
	//
	// If the socket is in blocking mode, EAGAIN should never occur.
	switch err {
	case syscall.EAGAIN, syscall.EINTR:
		// Not ready.
		return false
	default:
		// Ready whether there was error or no error.
		return true
	}
}

// sysToHeader converts a syscall.NlMsghdr to a Header.
func sysToHeader(r syscall.NlMsghdr) netlink.Header {
	// NB: the memory layout of Header and syscall.NlMsgHdr must be
	// exactly the same for this unsafe cast to work
	return *(*netlink.Header)(unsafe.Pointer(&r))
}
