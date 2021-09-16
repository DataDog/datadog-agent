// +build linux_bpf

package http

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/DataDog/gopsutil/process/so"
	"golang.org/x/sys/unix"
)

var sslProbes = []string{
	"uprobe/SSL_set_bio",
	"uprobe/SSL_set_fd",
	"uprobe/SSL_read",
	"uretprobe/SSL_read",
	"uprobe/SSL_write",
	"uprobe/SSL_shutdown",
}

const (
	sslSockByCtxMap = "ssl_sock_by_ctx"
	sslReadArgsMap  = "ssl_read_args"
	sslFDByBioMap   = "fd_by_ssl_bio"
)

var sslMaps = []string{
	httpInFlightMap,
	httpBatchesMap,
	httpBatchStateMap,
	string(probes.SockByPidFDMap),
	sslSockByCtxMap,
	sslReadArgsMap,
	sslFDByBioMap,
}

// sslProgram encapsulates the uprobe management for one specific OpenSSL library "instance"
// TODO: replace `Manager` by something lighter so we can avoid things such as parsing
// the eBPF ELF repeatedly
type sslProgram struct {
	mainProgram *ebpfProgram
	mgr         *manager.Manager
	offsets     []manager.ConstantEditor
	libPath     string
	uid         string
}

var _ subprogram = &sslProgram{}

func createSSLPrograms(mainProgram *ebpfProgram, offsets []manager.ConstantEditor, libraries []so.Library) []subprogram {
	var subprograms []subprogram
	for i, lib := range libraries {
		if !strings.Contains(lib.HostPath, "ssl") {
			continue
		}

		sslProg, err := newSSLProgram(mainProgram, offsets, lib.HostPath, i)
		if err != nil {
			log.Errorf("error creating SSL program for %s: %s", lib.HostPath, err)
			continue
		}

		subprograms = append(subprograms, sslProg)
	}

	return subprograms
}

func newSSLProgram(mainProgram *ebpfProgram, offsets []manager.ConstantEditor, libPath string, i int) (*sslProgram, error) {
	if libPath == "" {
		return nil, fmt.Errorf("path to libssl not provided")
	}

	uid := strconv.Itoa(i)
	var probes []*manager.Probe
	for _, sec := range sslProbes {
		probes = append(probes, &manager.Probe{
			Section:    sec,
			BinaryPath: libPath,
			UID:        uid,
		})
	}

	return &sslProgram{
		mainProgram: mainProgram,
		mgr:         &manager.Manager{Probes: probes},
		offsets:     offsets,
		libPath:     libPath,
		uid:         uid,
	}, nil
}

func (p *sslProgram) Init() error {
	var selectors []manager.ProbesSelector
	for _, sec := range sslProbes {
		selectors = append(selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: sec,
				UID:     p.uid,
			},
		})
	}

	log.Debugf("https (libssl) tracing enabled. lib=%s", p.libPath)
	return p.mgr.InitWithOptions(
		p.mainProgram.bytecode,
		manager.Options{
			RLimit: &unix.Rlimit{
				Cur: math.MaxUint64,
				Max: math.MaxUint64,
			},
			ActivatedProbes: selectors,
			ConstantEditors: p.offsets,
			MapEditors:      setupSharedMaps(p.mainProgram, sslMaps...),
		},
	)
}

func (p *sslProgram) Start() error {
	return p.mgr.Start()
}

func (p *sslProgram) Close() error {
	return p.mgr.Stop(manager.CleanAll)
}
