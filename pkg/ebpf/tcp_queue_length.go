// +build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"

	bpflib "github.com/iovisor/gobpf/bcc"
)

/*
#include <stdint.h>
*/
import "C"

type TCPQueueLengthTracer struct {
	m        *bpflib.Module
	queueMap *bpflib.Table
}

type QueueLength struct {
	Size C.int      `json:"size"`
	Min  C.uint32_t `json:"min"`
	Max  C.uint32_t `json:"max"`
}

type Stats struct {
	Pid    C.uint32_t  `json:"pid"`
	Rqueue QueueLength `json:"read queue"`
	Wqueue QueueLength `json:"write queue"`
}

type Conn struct {
	Saddr net.IP `json:"saddr"`
	Daddr net.IP `json:"daddr"`
	Sport uint16 `json:"sport"`
	Dport uint16 `json:"dport"`
}

type StatLine struct {
	Conn        Conn   `json:"conn"`
	ContainerID string `json:"containerid"`
	Stats       Stats  `json:"stats"`
}

type conn struct {
	Saddr C.uint32_t
	Daddr C.uint32_t
	Sport C.uint16_t
	Dport C.uint16_t
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

	kretprobe_recvmsg, err := m.LoadKprobe("kretprobe__tcp_recvmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kretprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKretprobe("tcp_recvmsg", kretprobe_recvmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_recvmsg: %s\n", err)
	}

	kprobe_sendmsg, err := m.LoadKprobe("kprobe__tcp_sendmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKprobe("tcp_sendmsg", kprobe_sendmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_sendmsg: %s\n", err)
	}

	kretprobe_sendmsg, err := m.LoadKprobe("kretprobe__tcp_sendmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kretprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKretprobe("tcp_sendmsg", kretprobe_sendmsg, -1); err != nil {
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
	if t == nil {
		return nil
	}

	var result []StatLine

	containerOfPID := make(map[C.uint32_t]string)

	for it := t.queueMap.Iter(); it.Next(); {
		var c conn
		var s Stats

		binary.Read(bytes.NewBuffer(it.Key()), binary.BigEndian, &c)
		binary.Read(bytes.NewBuffer(it.Leaf()), nativeEndian, &s)

		saddr := make(net.IP, 4)
		binary.BigEndian.PutUint32(saddr, uint32(c.Saddr))
		daddr := make(net.IP, 4)
		binary.BigEndian.PutUint32(daddr, uint32(c.Daddr))

		containerID, found := containerOfPID[s.Pid]
		if !found {
			containerID, err := metrics.ContainerIDForPID(int(s.Pid))
			if err == nil {
				containerOfPID[s.Pid] = containerID
			}
		}

		result = append(result, StatLine{
			Conn: Conn{
				Saddr: saddr,
				Daddr: daddr,
				Sport: uint16(c.Sport),
				Dport: uint16(c.Dport),
			},
			ContainerID: containerID,
			Stats:       s,
		})
	}

	return result
}

func (t *TCPQueueLengthTracer) GetAndFlush() []StatLine {
	result := t.Get()
	t.queueMap.DeleteAll()
	return result
}
