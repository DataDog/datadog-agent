// +build linux_bpf,bcc

package ebpf

import (
	"bufio"
	"bytes"
	// "encoding/binary"
	"fmt"
	// "net"
	"regexp"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/oomkill"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	bpflib "github.com/iovisor/gobpf/bcc"
)

/*
#include <string.h>
#include "c/oom-kill-kern-user.h"
*/
import "C"

type OOMKillProbe struct {
	m      *bpflib.Module
	oomMap *bpflib.Table
}

func NewOOMKillProbe() (*OOMKillProbe, error) {
	source_raw, err := bytecode.Asset("oom-kill-kern.c")
	if err != nil {
		return nil, fmt.Errorf("Couldn’t find asset “oom-kill-kern.c”: %v", err)
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
			header, err := bytecode.Asset(string(match[1]))
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
		return nil, fmt.Errorf("failed to compile “oom-kill-kern.c”")
	}

	kprobe, err := m.LoadKprobe("kprobe__oom_kill_process")
	if err != nil {
		return nil, fmt.Errorf("failed to load kprobe__oom_kill_process: %s\n", err)
	}

	if err := m.AttachKprobe("oom_kill_process", kprobe, -1); err != nil {
		return nil, fmt.Errorf("failed to attach oom_kill_process: %s\n", err)
	}

	table := bpflib.NewTable(m.TableId("oomStats"), m)

	return &OOMKillProbe{
		m:      m,
		oomMap: table,
	}, nil
}

func (k *OOMKillProbe) Close() {
	k.m.Close()
}

func (k *OOMKillProbe) GetAndFlush() []oomkill.Stats {
	results := k.Get()
	k.oomMap.DeleteAll()
	return results
}

func (k *OOMKillProbe) Get() []oomkill.Stats {
	if k == nil {
		return nil
	}

	var results []oomkill.Stats

	for it := k.oomMap.Iter(); it.Next(); {
		var stat C.struct_oom_stats

		data := it.Leaf()
		C.memcpy(unsafe.Pointer(&stat), unsafe.Pointer(&data[0]), C.sizeof_struct_oom_stats)

		results = append(results, convertStats(stat))
	}
	log.Infof("CELENE inside OOMKillProbe.Get() results is %v", results)
	return results
}

func convertStats(in C.struct_oom_stats) (out oomkill.Stats) {
	log.Infof("CELENE inside convertStats ContainerID is %s", C.GoString(&in.cgroup_name[0]))
	log.Infof("CELENE inside convertStats Pid is %d", uint32(in.pid))
	log.Infof("CELENE inside convertStats TPid is %d", uint32(in.tpid))

	out.ContainerID = C.GoString(&in.cgroup_name[0])
	out.Pid = uint32(in.pid)
	out.TPid = uint32(in.tpid)
	out.FComm = C.GoString(&in.fcomm[0])
	out.TComm = C.GoString(&in.tcomm[0])
	out.Pages = uint64(in.pages)
	out.MemCgOOM = uint32(in.memcg_oom)
	return
}
