// +build linux
// +build !android

package netlink

import (
	"errors"
	"math"
	"os"
	"syscall"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

var _ netlink.Socket = &Socket{}
var errNotImplemented = errors.New("not implemented")

// Socket is an implementation of netlink.Socket (github.com/mdlayher/netlink)
// It's mostly a copy of the original implementation (netlink.conn) with a few optimizations:
// * We don't MSG_PEEK as we use a pre-allocated buffer large enough to fit any netlink message;
// * We use a buffer pool for the message data;
// * We remove all the synchronization & go-channels cruft and bring it upstream in a cheaper/simpler way (Consumer)
type Socket struct {
	fd   *os.File
	pid  uint32
	conn syscall.RawConn

	// A 32KB buffer which we use for polling the socket.
	// Since a netlink message can't exceed that size
	// (in *theory* they can be as large as 4GB (u32), but see link below)
	// we can avoid message peeks and and essentially cut recvmsg syscalls by half
	// which is currently a perf bottleneck in certain workloads.
	// https://www.spinics.net/lists/netdev/msg431592.html
	recvbuf []byte
}

// NewSocket creates a new NETLINK socket
func NewSocket() (*Socket, error) {
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

	socket := &Socket{
		fd:      file,
		pid:     pid,
		conn:    conn,
		recvbuf: make([]byte, 32*1024),
	}
	return socket, nil
}

// Send a netlink.Message
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

// Receive is not implemented. See ReceiveInto
func (s *Socket) Receive() ([]netlink.Message, error) {
	return nil, errNotImplemented
}

// ReceiveInto reads one or more netlink.Messages off the socket
func (s *Socket) ReceiveInto(b []byte) ([]netlink.Message, int32, error) {
	oob := make([]byte, unix.CmsgSpace(24))
	n, oobn, err := s.recvmsg(s.recvbuf, oob, 0)
	if err != nil {
		return nil, 0, os.NewSyscallError("recvmsg", err)
	}

	n = nlmsgAlign(n)
	// If we cannot fit the date into the suplied buffer,  we allocate a slice
	// with enough capacity. This should happen very rarely.
	if n > len(b) {
		b = make([]byte, n)
	}
	copy(b, s.recvbuf[:n])

	raw, err := syscall.ParseNetlinkMessage(b[:n])
	if err != nil {
		return nil, 0, err
	}

	msgs := make([]netlink.Message, 0, len(raw))
	for _, r := range raw {
		m := netlink.Message{
			Header: sysToHeader(r.Header),
			Data:   r.Data,
		}

		msgs = append(msgs, m)
	}

	var netns int32
	if oobn > 0 {
		oob = oob[:oobn]
		scms, err := unix.ParseSocketControlMessage(oob)
		if err != nil {
			return nil, 0, err
		}

		netns = parseNetNS(scms)
	}

	return msgs, netns, nil
}

func parseNetNS(scms []unix.SocketControlMessage) int32 {
	for _, m := range scms {
		if m.Header.Level != unix.SOL_NETLINK || m.Header.Type != unix.NETLINK_LISTEN_ALL_NSID {
			continue
		}

		return *(*int32)(unsafe.Pointer(&m.Data[0]))
	}

	return 0
}

// File descriptor of the socket
func (s *Socket) File() *os.File {
	return s.fd
}

// Close the socket
func (s *Socket) Close() error {
	return s.fd.Close()
}

// SendMessages isn't implemented in our case
func (s *Socket) SendMessages(m []netlink.Message) error {
	return errNotImplemented
}

// JoinGroup creates a new group membership
func (s *Socket) JoinGroup(group uint32) error {
	return os.NewSyscallError("setsockopt", s.SetSockoptInt(
		unix.SOL_NETLINK,
		unix.NETLINK_ADD_MEMBERSHIP,
		int(group),
	))
}

// LeaveGroup deletes a group membership
func (s *Socket) LeaveGroup(group uint32) error {
	return os.NewSyscallError("setsockopt", s.SetSockoptInt(
		unix.SOL_NETLINK,
		unix.NETLINK_DROP_MEMBERSHIP,
		int(group),
	))
}

// SetSockoptInt sets a socket option
func (s *Socket) SetSockoptInt(level, opt, value int) error {
	// Value must be in range of a C integer.
	if value < math.MinInt32 || value > math.MaxInt32 {
		return unix.EINVAL
	}

	var err error
	doErr := s.conn.Control(func(fd uintptr) {
		err = unix.SetsockoptInt(int(fd), level, opt, value)
	})

	if doErr != nil {
		return doErr
	}

	return err
}

// GetSockoptInt gets a socket option
func (s *Socket) GetSockoptInt(level, opt int) (int, error) {
	var err error
	var v int
	doErr := s.conn.Control(func(fd uintptr) {
		v, err = unix.GetsockoptInt(int(fd), level, opt)
	})

	if doErr != nil {
		return v, doErr
	}

	return v, err
}

// SetBPF attaches an assembled BPF program to the socket
func (s *Socket) SetBPF(filter []bpf.RawInstruction) error {
	prog := unix.SockFprog{
		Len:    uint16(len(filter)),
		Filter: (*unix.SockFilter)(unsafe.Pointer(&filter[0])),
	}

	var err error
	ctrlErr := s.conn.Control(func(fd uintptr) {
		err = unix.SetsockoptSockFprog(int(fd), unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &prog)
	})
	if ctrlErr != nil {
		return ctrlErr
	}

	return err
}

func (s *Socket) recvmsg(b []byte, oob []byte, flags int) (int, int, error) {
	var (
		n    int
		oobn int
		err  error
	)

	ctrlErr := s.conn.Read(func(fd uintptr) bool {
		n, oobn, _, _, err = unix.Recvmsg(int(fd), b, oob, flags)
		return ready(err)
	})

	if ctrlErr != nil {
		return 0, 0, ctrlErr
	}

	return n, oobn, err
}

// Copied from github.com/mdlayher/netlink
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
