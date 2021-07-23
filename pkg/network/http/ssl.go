// +build linux_bpf

package http

import (
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

// these maps are shared among the "main" HTTP (socket-filter) program and
// the (uprobe-based) OpenSSL programs
var httpMaps = []probes.BPFMapName{
	probes.HttpInFlightMap,
	probes.HttpBatchesMap,
	probes.HttpBatchStateMap,
}

var _ subprogram = &sslProgram{}

// sslProgram encapsulates the uprobe management for one specific OpenSSL library "instance"
// TODO: replace `Manager` by something lighter so we can avoid things such as parsing
// the eBPF ELF repeatedly
type sslProgram struct {
	mainProgram *ebpfProgram
	mgr         *manager.Manager
	offsets     []manager.ConstantEditor
	sockFDMap   *ebpf.Map
	libPath     string
}

func newSSLProgram(mainProgram *ebpfProgram, offsets []manager.ConstantEditor, sockFD *ebpf.Map, libPath string) (*sslProgram, error) {
	if sockFD == nil {
		return nil, fmt.Errorf("sockFD map not provided")
	}

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{Section: "uprobe/SSL_set_fd", BinaryPath: libPath},
			{Section: "uprobe/SSL_read", BinaryPath: libPath},
			{Section: "uretprobe/SSL_read", BinaryPath: libPath},
			{Section: "uprobe/SSL_write", BinaryPath: libPath},
			{Section: "uprobe/SSL_shutdown", BinaryPath: libPath},
		},
	}

	return &sslProgram{
		mainProgram: mainProgram,
		mgr:         mgr,
		offsets:     offsets,
		sockFDMap:   sockFD,
		libPath:     libPath,
	}, nil
}

func (p *sslProgram) Init() error {
	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: "uprobe/SSL_set_fd",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: "uprobe/SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: "uretprobe/SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: "uprobe/SSL_write",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: "uprobe/SSL_shutdown",
				},
			},
		},
		ConstantEditors: p.offsets,
		MapEditors:      make(map[string]*ebpf.Map),
	}

	// set up shared maps
	// * the sockFD is shared with the network-tracer program;
	// * all other HTTP-specific maps are shared with the "core" HTTP program;
	options.MapEditors["sock_by_pid_fd"] = p.sockFDMap
	for _, mapName := range httpMaps {
		name := string(mapName)
		m, _, _ := p.mainProgram.GetMap(name)
		if m == nil {
			return fmt.Errorf("couldn't retrieve map: %s", m)
		}
		options.MapEditors[name] = m
	}

	log.Debugf("tracing SSL library located at %s", p.libPath)
	return p.mgr.InitWithOptions(p.mainProgram.bytecode, options)
}

func (p *sslProgram) Start() error {
	return p.mgr.Start()
}

func (p *sslProgram) Close() error {
	return p.mgr.Stop(manager.CleanAll)
}
