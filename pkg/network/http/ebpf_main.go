// +build linux_bpf

package http

import (
	"math"
	"os"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

const (
	httpInFlightMap      = "http_in_flight"
	httpBatchesMap       = "http_batches"
	httpBatchStateMap    = "http_batch_state"
	httpNotificationsMap = "http_notifications"
	sslSockByCtxMap      = "ssl_sock_by_ctx"

	// ELF section of the BPF_PROG_TYPE_SOCKET_FILTER program used
	// to inspect plain HTTP traffic
	httpSocketFilter = "socket/http_filter"

	// maxActive configures the maximum number of instances of the
	// kretprobe-probed functions handled simultaneously.  This value should be
	// enough for typical workloads (e.g. some amount of processes blocked on
	// the accept syscall).
	maxActive = 128

	// size of the channel containing completed http_notification_objects
	batchNotificationsChanSize = 100

	// UID used to create the base probes
	baseUID = "base"
)

type ebpfProgram struct {
	*manager.Manager
	cfg         *config.Config
	perfHandler *ddebpf.PerfHandler
	bytecode    bytecode.AssetReader
	sockFDMap   *ebpf.Map
	offsets     []manager.ConstantEditor
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map) (*ebpfProgram, error) {
	bytecode, err := netebpf.ReadHTTPModule(c.BPFDir, c.BPFDebug)
	if err != nil {
		return nil, err
	}

	perfHandler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: httpNotificationsMap},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        perfHandler.DataHandler,
					LostHandler:        perfHandler.LostHandler,
				},
			},
		},
		Probes: []*manager.Probe{
			{Section: httpSocketFilter},
			{Section: string(probes.TCPSendMsgReturn), KProbeMaxActive: maxActive},
		},
	}

	// Load SSL & Crypto probes
	var extraProbes []string
	extraProbes = append(extraProbes, sslProbes)
	extraProbes = append(extraProbes, cryptoProbes)
	for _, sec := range extraProbes {
		mgr.Probes = append(mgr.Probes, &manager.Probe{
			Section: sec,
			UID:     baseUID,
		})
	}

	program := &ebpfProgram{
		Manager:     mgr,
		perfHandler: perfHandler,
		bytecode:    bytecode,
		cfg:         c,
		sockFDMap:   sockFD,
		offsets:     offsets,
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			httpInFlightMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
			sslSockByCtxMap: {
				Type:       ebpf.Hash,
				MaxEntries: uint32(e.cfg.MaxTrackedConnections),
				EditorFlag: manager.EditMaxEntries,
			},
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: httpSocketFilter,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probes.TCPSendMsgReturn),
				},
			},
		},
		ConstantEditors: e.offsets,
	}

	if e.sockFDMap != nil {
		options.MapEditors = map[string]*ebpf.Map{
			string(probes.SockByPidFDMap): e.sockFDMap,
		}
	}

	err := e.InitWithOptions(e.bytecode, options)
	if err != nil {
		return err
	}

	initSSLTracing(e.Manager, e.cfg)
	return nil
}

func (e *ebpfProgram) Close() error {
	return e.Manager.Stop(manager.CleanAll)
}
