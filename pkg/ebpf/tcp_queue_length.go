// +build linux_bpf

package ebpf

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"unsafe"

	bpflib "github.com/iovisor/gobpf/bcc"
)

/*
#include <string.h>
#include "c/tcp_queue_length_kern_user.h"
*/
import "C"

type TCPQueueLengthTracer struct {
	m        *bpflib.Module
	queueMap *bpflib.Table
}

func NewTCPQueueLengthTracer() (*TCPQueueLengthTracer, error) {
	source_raw, err := Asset("tcp_queue_length_kern.c")
	if err != nil {
		return nil, fmt.Errorf("Couldn’t find asset “tcp_queue_length.c”: %v", err)
	}

	// Process the `#include` of embedded headers.
	// Note that embedded headers including other embedded headers is not managed because
	// this would also require to properly handle inclusion guards.
	includeRegexp := regexp.MustCompile(`^\s*#\s*include\s+"(.*)"$`)
	var source bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewBuffer(source_raw))
	for scanner.Scan() {
		match := includeRegexp.FindSubmatch(scanner.Bytes())
		if len(match) == 2 {
			header, err := Asset(string(match[1]))
			if err == nil {
				source.Write(header)
				continue
			}
		}
		source.Write(scanner.Bytes())
		source.WriteByte('\n')
	}

	m := bpflib.NewModule(source.String(), []string{})
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

func (t *TCPQueueLengthTracer) Get() []Stats {
	if t == nil {
		return nil
	}

	var result []Stats

	for it := t.queueMap.Iter(); it.Next(); {
		var in C.struct_stats // kernel       <-> system-probe
		var out Stats         // system-probe <-> agent

		// `binary.Read(…)` doesn’t work because reflection doesn’t work with C types.
		// binary.Read(bytes.NewBuffer(it.Leaf()), bpflib.GetHostByteOrder(), &in)

		data := it.Leaf()
		C.memcpy(unsafe.Pointer(&in), unsafe.Pointer(&data[0]), C.sizeof_struct_stats)

		// TODO: Can this code be handled by using reflection? Would it be clearer?
		out.Pid = uint32(in.pid)
		out.ContainerID = C.GoString(&in.cgroup_name[0])
		out.Conn.Saddr = make(net.IP, 4)
		bpflib.GetHostByteOrder().PutUint32(out.Conn.Saddr, uint32(in.conn.saddr))
		out.Conn.Daddr = make(net.IP, 4)
		bpflib.GetHostByteOrder().PutUint32(out.Conn.Daddr, uint32(in.conn.daddr))
		port := make([]byte, 2)
		bpflib.GetHostByteOrder().PutUint16(port, uint16(in.conn.dport))
		out.Conn.Dport = binary.BigEndian.Uint16(port)
		bpflib.GetHostByteOrder().PutUint16(port, uint16(in.conn.sport))
		out.Conn.Sport = binary.BigEndian.Uint16(port)
		out.Rqueue.Size = int(in.rqueue.size)
		out.Rqueue.Min = uint32(in.rqueue.min)
		out.Rqueue.Max = uint32(in.rqueue.max)
		out.Wqueue.Size = int(in.wqueue.size)
		out.Wqueue.Min = uint32(in.wqueue.min)
		out.Wqueue.Max = uint32(in.wqueue.max)

		result = append(result, out)
	}

	return result
}

func (t *TCPQueueLengthTracer) GetAndFlush() []Stats {
	result := t.Get()
	t.queueMap.DeleteAll()
	return result
}
