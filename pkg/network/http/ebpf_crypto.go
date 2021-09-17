// +build linux_bpf

package http

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/DataDog/gopsutil/process/so"
	"golang.org/x/sys/unix"
)

var cryptoProbes = []string{
	"uprobe/BIO_new_socket",
	"uretprobe/BIO_new_socket",
}

const (
	cryptoNewSocketArgsMap = "bio_new_socket_args"
)

var cryptoMaps = []string{
	cryptoNewSocketArgsMap,
	sslFDByBioMap,
}

type cryptoProgram struct {
	mainProgram *ebpfProgram
	mgr         *manager.Manager
	libPath     string
	uid         string
}

var _ subprogram = &cryptoProgram{}

func createCryptoPrograms(mainProgram *ebpfProgram, libraries []so.Library) []subprogram {
	var subprograms []subprogram
	for i, lib := range libraries {
		if !strings.Contains(lib.HostPath, "crypto") {
			continue
		}

		p, err := newCryptoProgram(mainProgram, lib.HostPath, i)
		if err != nil {
			log.Errorf("error creating crypto program to trace %s: %s", lib.HostPath, err)
			continue
		}

		subprograms = append(subprograms, p)
	}

	return subprograms
}

func newCryptoProgram(mainProgram *ebpfProgram, libPath string, i int) (*cryptoProgram, error) {
	if libPath == "" {
		return nil, fmt.Errorf("path to libcrypto not provided")
	}

	uid := strconv.Itoa(i)
	var probes []*manager.Probe
	for _, sec := range cryptoProbes {
		probes = append(probes, &manager.Probe{
			Section:    sec,
			BinaryPath: libPath,
			UID:        uid,
		})
	}

	return &cryptoProgram{
		mainProgram: mainProgram,
		mgr:         &manager.Manager{Probes: probes},
		libPath:     libPath,
		uid:         uid,
	}, nil
}

func (p *cryptoProgram) Init() error {
	var selectors []manager.ProbesSelector
	for _, sec := range cryptoProbes {
		selectors = append(selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: sec,
				UID:     p.uid,
			},
		})
	}

	log.Debugf("https (libcrypto) tracing enabled. library=%s", p.libPath)
	return p.mgr.InitWithOptions(
		p.mainProgram.bytecode,
		manager.Options{
			RLimit: &unix.Rlimit{
				Cur: math.MaxUint64,
				Max: math.MaxUint64,
			},
			ActivatedProbes: selectors,
			MapEditors:      setupSharedMaps(p.mainProgram, cryptoMaps...),
		},
	)
}

func (p *cryptoProgram) Start() error {
	return p.mgr.Start()
}

func (p *cryptoProgram) Close() error {
	return p.mgr.Stop(manager.CleanAll)
}
