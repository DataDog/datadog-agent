// +build linux_bpf

package http

import (
	"math"
	"os"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"golang.org/x/sys/unix"
)

const (
	httpInFlightMap          = "http_in_flight"
	httpBatchesMap           = "http_batches"
	httpBatchStateMap        = "http_batch_state"
	httpNotificationsPerfMap = "http_notifications"

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
)

type ebpfProgram struct {
	*manager.Manager
	cfg         *config.Config
	bytecode    bytecode.AssetReader
	offsets     []manager.ConstantEditor
	subprograms []subprogram

	batchCompletionHandler *ddebpf.PerfHandler
}

type subprogram interface {
	ConfigureManager(*manager.Manager)
	ConfigureOptions(*manager.Options)
	Start()
	Stop()
}

func newEBPFProgram(c *config.Config, offsets []manager.ConstantEditor, sockFD *ebpf.Map) (*ebpfProgram, error) {
	bytecode, err := getRuntimeCompiledHTTP(c)
	if err != nil {
		return nil, err
	}

	batchCompletionHandler := ddebpf.NewPerfHandler(batchNotificationsChanSize)
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: httpInFlightMap},
			{Name: httpBatchesMap},
			{Name: httpBatchStateMap},
			{Name: sslSockByCtxMap},
			{Name: "ssl_read_args"},
			{Name: "bio_new_socket_args"},
			{Name: "fd_by_ssl_bio"},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: httpNotificationsPerfMap},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        batchCompletionHandler.DataHandler,
					LostHandler:        batchCompletionHandler.LostHandler,
				},
			},
		},
		Probes: []*manager.Probe{
			{Section: httpSocketFilter},
			{Section: string(probes.TCPSendMsgReturn), KProbeMaxActive: maxActive},
		},
	}

	openSSLProgram, _ := newOpenSSLProgram(c, sockFD)
	program := &ebpfProgram{
		Manager:                mgr,
		bytecode:               bytecode,
		cfg:                    c,
		offsets:                offsets,
		batchCompletionHandler: batchCompletionHandler,
		subprograms:            []subprogram{openSSLProgram},
	}

	return program, nil
}

func (e *ebpfProgram) Init() error {
	for _, s := range e.subprograms {
		s.ConfigureManager(e.Manager)
	}
	setupDumpHandler(e.Manager)

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

	for _, s := range e.subprograms {
		s.ConfigureOptions(&options)
	}

	err := e.InitWithOptions(e.bytecode, options)
	if err != nil {
		return err
	}

	return nil
}

func (e *ebpfProgram) Start() error {
	err := e.Manager.Start()
	if err != nil {
		return err
	}

	for _, s := range e.subprograms {
		s.Start()
	}

	return nil
}

func (e *ebpfProgram) Close() error {
	err := e.Manager.Stop(manager.CleanAll)
	e.batchCompletionHandler.Stop()
	for _, s := range e.subprograms {
		s.Stop()
	}
	return err
}
