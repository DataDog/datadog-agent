// +build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	bpflib "github.com/iovisor/gobpf/bcc"
)

type TCPQueueLengthTracer struct {
	m        *bpflib.Module
	queueMap *bpflib.Table
}

type QueueLength struct {
	Min uint32
	Max uint32
}

type Stats struct {
	Rqueue QueueLength
	Wqueue QueueLength
}

type Conn struct {
	Saddr net.IP
	Daddr net.IP
	Sport uint16
	Dport uint16
}

type StatLine struct {
	Conn  Conn
	Stats Stats
}

type conn struct {
	Saddr uint32
	Daddr uint32
	Sport uint16
	Dport uint16
}

func NewTCPQueueLengthTracer() (*TCPQueueLengthTracer, error) {
	source, err := Asset("tcp_queue_length_kern.c")
	if err != nil {
		return nil, fmt.Errorf("Couldn’t find asset “tcp_queue_length.c”: %v", err)
	}

	m := bpflib.NewModule(string(source), []string{})
	if m == nil {
		return nil, fmt.Errorf("Failed to compile “tcp_queue_length.c”")
	}

	kprobe_recvmsg, err := m.LoadKprobe("kprobe__tcp_recvmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKprobe("tcp_recvmsg", kprobe_recvmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_recvmsg: %s\n", err)
	}

	kprobe_sendmsg, err := m.LoadKprobe("kprobe__tcp_sendmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKprobe("tcp_sendmsg", kprobe_sendmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_sendmsg: %s\n", err)
	}

	table := bpflib.NewTable(m.TableId("queue"), m)

	return &TCPQueueLengthTracer{
		m:        m,
		queueMap: table,
	}, nil
}

func (t *TCPQueueLengthTracer) Close() {
	t.m.Close()
}

func (t *TCPQueueLengthTracer) Get() []StatLine {
	var result []StatLine

	for it := t.queueMap.Iter(); it.Next(); {
		var c conn
		var s Stats

		binary.Read(bytes.NewBuffer(it.Key()), binary.BigEndian, &c)
		binary.Read(bytes.NewBuffer(it.Leaf()), nativeEndian, &s)

		saddr := make(net.IP, 4)
		binary.BigEndian.PutUint32(saddr, c.Saddr)
		daddr := make(net.IP, 4)
		binary.BigEndian.PutUint32(daddr, c.Daddr)

		result = append(result, StatLine{
			Conn: Conn{
				Saddr: saddr,
				Daddr: daddr,
				Sport: c.Sport, //strconv.Itoa(int(c.Sport)),
				Dport: c.Dport, //strconv.Itoa(int(c.Dport)),
			},
			Stats: s,
		})
	}

	return result
}

func (t *TCPQueueLengthTracer) GetAndFlush() []StatLine {
	result := t.Get()
	t.queueMap.DeleteAll()
	return result
}
