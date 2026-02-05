// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/davecgh/go-spew/spew"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	sslReadExProbe              = "uprobe__SSL_read_ex"
	sslReadExRetprobe           = "uretprobe__SSL_read_ex"
	sslWriteExProbe             = "uprobe__SSL_write_ex"
	sslWriteExRetprobe          = "uretprobe__SSL_write_ex"
	sslDoHandshakeProbe         = "uprobe__SSL_do_handshake"
	sslDoHandshakeRetprobe      = "uretprobe__SSL_do_handshake"
	sslConnectProbe             = "uprobe__SSL_connect"
	sslConnectRetprobe          = "uretprobe__SSL_connect"
	sslSetBioProbe              = "uprobe__SSL_set_bio"
	sslSetFDProbe               = "uprobe__SSL_set_fd"
	sslReadProbe                = "uprobe__SSL_read"
	sslReadRetprobe             = "uretprobe__SSL_read"
	sslWriteProbe               = "uprobe__SSL_write"
	sslWriteRetprobe            = "uretprobe__SSL_write"
	sslShutdownProbe            = "uprobe__SSL_shutdown"
	bioNewSocketProbe           = "uprobe__BIO_new_socket"
	bioNewSocketRetprobe        = "uretprobe__BIO_new_socket"
	bioFreeProbe                = "uprobe__BIO_free"
	gnutlsHandshakeProbe        = "uprobe__gnutls_handshake"
	gnutlsHandshakeRetprobe     = "uretprobe__gnutls_handshake"
	gnutlsTransportSetInt2Probe = "uprobe__gnutls_transport_set_int2"
	gnutlsTransportSetPtrProbe  = "uprobe__gnutls_transport_set_ptr"
	gnutlsTransportSetPtr2Probe = "uprobe__gnutls_transport_set_ptr2"
	gnutlsRecordRecvProbe       = "uprobe__gnutls_record_recv"
	gnutlsRecordRecvRetprobe    = "uretprobe__gnutls_record_recv"
	gnutlsRecordSendProbe       = "uprobe__gnutls_record_send"
	gnutlsRecordSendRetprobe    = "uretprobe__gnutls_record_send"
	gnutlsByeProbe              = "uprobe__gnutls_bye"
	gnutlsDeinitProbe           = "uprobe__gnutls_deinit"

	rawTracepointSchedProcessExit = "raw_tracepoint__sched_process_exit"
	oldTracepointSchedProcessExit = "tracepoint__sched__sched_process_exit"
	kprobeDoExit                  = "kprobe__do_exit"

	// UsmTLSAttacherName holds the name used for the uprobe attacher of tls programs. Used for tests.
	UsmTLSAttacherName = "usm_tls"

	sslSockByCtxMap     = "ssl_sock_by_ctx"
	sslCtxByTupleMap    = "ssl_ctx_by_tuple"
	sslCtxByPIDTGIDMap  = "ssl_ctx_by_pid_tgid"
	sslReadArgsMap      = "ssl_read_args"
	sslReadExArgsMap    = "ssl_read_ex_args"
	sslWriteArgsMap     = "ssl_write_args"
	sslWriteExArgsMap   = "ssl_write_ex_args"
	bioNewSocketArgsMap = "bio_new_socket_args"
	fdBySSLBioMap       = "fd_by_ssl_bio"
)

// pidKeyedTLSMaps is the single source of truth for all TLS eBPF maps that use pid_tgid as keys.
//
// IMPORTANT: Map names must be unique within their first 15 characters due to kernel truncation
// (BPF_OBJ_NAME_LEN - 1). The leak detection system searches maps by truncated names, so names
// like "hash_map_name_10" and "hash_map_name_11" would collide as both truncate to "hash_map_name_1".
//
// When adding a new PID-keyed map:
// 1. Add a new field to this struct with the map name as the value
// 2. Ensure the name is unique within the first 15 characters
// 3. Add the corresponding map cleaner field to sslProgram struct
// 4. Initialize it in initAllMapCleaners() with uint64 key type
// The GetPIDKeyedTLSMapNames() function will automatically include it via reflection.
var pidKeyedTLSMaps = struct {
	SSLReadArgs      string
	SSLReadExArgs    string
	SSLWriteArgs     string
	SSLWriteExArgs   string
	BioNewSocketArgs string
	SSLCtxByPIDTGID  string
}{
	SSLReadArgs:      "ssl_read_args",
	SSLReadExArgs:    "ssl_read_ex_args",
	SSLWriteArgs:     "ssl_write_args",
	SSLWriteExArgs:   "ssl_write_ex_args",
	BioNewSocketArgs: "bio_new_socket_args",
	SSLCtxByPIDTGID:  "ssl_ctx_by_pid_tgid",
}

// GetPIDKeyedTLSMapNames returns the names of all TLS eBPF maps that use pid_tgid as keys.
// It uses reflection to extract all field values from pidKeyedTLSMaps struct.
// This ensures the list is automatically kept in sync with the struct definition.
func GetPIDKeyedTLSMapNames() []string {
	v := reflect.ValueOf(pidKeyedTLSMaps)
	names := make([]string, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		names[i] = v.Field(i).String()
	}
	return names
}

var openSSLProbes = []manager.ProbesSelector{
	&manager.BestEffort{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslReadExProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslReadExRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslWriteExProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslWriteExRetprobe,
				},
			},
		},
	},
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslDoHandshakeProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslDoHandshakeRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslConnectProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslConnectRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslSetBioProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslSetFDProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslReadProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslReadRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslWriteProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslWriteRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: sslShutdownProbe,
				},
			},
		},
	},
}

var cryptoProbes = []manager.ProbesSelector{
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: bioNewSocketProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: bioNewSocketRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: bioFreeProbe,
				},
			},
		},
	},
}

var gnuTLSProbes = []manager.ProbesSelector{
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsHandshakeProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsHandshakeRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsTransportSetInt2Probe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsTransportSetPtrProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsTransportSetPtr2Probe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsRecordRecvProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsRecordRecvRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsRecordSendProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsRecordSendRetprobe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsByeProbe,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: gnutlsDeinitProbe,
				},
			},
		},
	},
}

var sharedLibrariesMaps = []*manager.Map{
	{
		Name: sslSockByCtxMap,
	},
	{
		Name: sslCtxByTupleMap,
	},
	{
		Name: pidKeyedTLSMaps.SSLReadArgs,
	},
	{
		Name: pidKeyedTLSMaps.SSLReadExArgs,
	},
	{
		Name: pidKeyedTLSMaps.SSLWriteArgs,
	},
	{
		Name: pidKeyedTLSMaps.SSLWriteExArgs,
	},
	{
		Name: pidKeyedTLSMaps.BioNewSocketArgs,
	},
	{
		Name: fdBySSLBioMap,
	},
	{
		Name: pidKeyedTLSMaps.SSLCtxByPIDTGID,
	},
}

// Template, will be modified during runtime.
// The constructor of SSLProgram requires more parameters than we provide in the general way, thus we need to have
// a dynamic initialization.
var opensslSpec = &protocols.ProtocolSpec{
	Factory: newSSLProgramProtocolFactory,
	Maps:    sharedLibrariesMaps,
	Probes: []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__tcp_sendmsg",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadExProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadExRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteExProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteExRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslDoHandshakeProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslDoHandshakeRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslConnectProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslConnectRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslSetBioProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslSetFDProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslReadRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslWriteRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: sslShutdownProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: bioNewSocketProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: bioNewSocketRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: bioFreeProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsHandshakeProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsHandshakeRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsTransportSetInt2Probe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsTransportSetPtrProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsTransportSetPtr2Probe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsRecordRecvProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsRecordRecvRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsRecordSendProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsRecordSendRetprobe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsByeProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: gnutlsDeinitProbe,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: rawTracepointSchedProcessExit,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: oldTracepointSchedProcessExit,
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: kprobeDoExit,
			},
		},
	},
}

var (
	// The interval of the periodic scan for terminated processes. Increasing the interval, might cause larger spikes in cpu
	// and lowering it might cause constant cpu usage.
	// Defined as a var to allow tests to override it.
	nativeTLSScanTerminatedProcessesInterval = 30 * time.Second
)

type sslProgram struct {
	cfg         *config.Config
	attacher    *uprobes.UprobeAttacher
	ebpfManager *manager.Manager

	sslReadArgsMapCleaner      *ddebpf.MapCleaner[uint64, http.SslReadArgs]
	sslReadExArgsMapCleaner    *ddebpf.MapCleaner[uint64, http.SslReadExArgs]
	sslWriteArgsMapCleaner     *ddebpf.MapCleaner[uint64, http.SslWriteArgs]
	sslWriteExArgsMapCleaner   *ddebpf.MapCleaner[uint64, http.SslWriteExArgs]
	bioNewSocketArgsMapCleaner *ddebpf.MapCleaner[uint64, uint32]

	sslCtxByPIDTGIDMapCleaner *ddebpf.MapCleaner[uint64, uint64]
	sslSockByCtxMapCleaner    *ddebpf.MapCleaner[http.SSLCtxPidTGID, http.SslSock]
	sslCtxByTupleMapCleaner   *ddebpf.MapCleaner[http.ConnTuple, uint64]
}

func newSSLProgramProtocolFactory(m *manager.Manager, c *config.Config) (protocols.Protocol, error) {
	if !c.EnableNativeTLSMonitoring || !usmconfig.TLSSupported(c) || !usmconfig.UretprobeSupported() {
		return nil, nil
	}

	procRoot := kernel.ProcFSRoot()

	rules := []*uprobes.AttachRule{
		{
			Targets:          uprobes.AttachToSharedLibraries,
			ProbesSelector:   openSSLProbes,
			LibraryNameRegex: regexp.MustCompile(`libssl.so`),
		},
		{
			Targets:          uprobes.AttachToSharedLibraries,
			ProbesSelector:   cryptoProbes,
			LibraryNameRegex: regexp.MustCompile(`libcrypto.so`),
		},
		{
			Targets:          uprobes.AttachToSharedLibraries,
			ProbesSelector:   gnuTLSProbes,
			LibraryNameRegex: regexp.MustCompile(`libgnutls.so`),
		},
	}
	o := &sslProgram{
		cfg:         c,
		ebpfManager: m,
	}
	attacherConfig := uprobes.AttacherConfig{
		ProcRoot:                       procRoot,
		Rules:                          rules,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		EbpfConfig:                     &c.Config,
		PerformInitialScan:             true,
		EnablePeriodicScanNewProcesses: true,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		ScanProcessesInterval:          nativeTLSScanTerminatedProcessesInterval,
		EnableDetailedLogging:          false,
		OnSyncCallback:                 o.cleanupDeadPids,
	}

	var err error
	o.attacher, err = uprobes.NewUprobeAttacher(consts.USMModuleName, UsmTLSAttacherName, attacherConfig, m, uprobes.NopOnAttachCallback, uprobes.AttacherDependencies{
		Inspector:      &uprobes.NativeBinaryInspector{},
		ProcessMonitor: monitor.GetProcessMonitor(),
		Telemetry:      telemetry.GetCompatComponent(),
	})
	if err != nil {
		return nil, fmt.Errorf("error initializing uprobes attacher: %s", err)
	}

	return o, nil
}

// GetStats is a no-op.
func (o *sslProgram) GetStats() (*protocols.ProtocolStats, func()) {
	return nil, nil
}

// Name return the program's name.
func (o *sslProgram) Name() string {
	return "openssl"
}

func sharedLibrariesConfigureOptions(options *manager.Options, cfg *config.Config) {
	options.MapSpecEditors[sslSockByCtxMap] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[sslCtxByTupleMap] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
	options.MapSpecEditors[pidKeyedTLSMaps.SSLCtxByPIDTGID] = manager.MapSpecEditor{
		MaxEntries: cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}
	options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__tcp_sendmsg"},
	})
}

// ConfigureOptions changes map attributes to the given options.
func (o *sslProgram) ConfigureOptions(options *manager.Options) {
	sharedLibrariesConfigureOptions(options, o.cfg)
	o.addProcessExitProbe(options)
}

// initMapCleaner creates and assigns a MapCleaner for the given eBPF map.
func initMapCleaner[K, V interface{}](mgr *manager.Manager, mapName, attacherName string) (*ddebpf.MapCleaner[K, V], error) {
	mapObj, _, err := mgr.GetMap(mapName)
	if err != nil {
		return nil, fmt.Errorf("dead process ssl cleaner failed to get map: %q error: %w", mapName, err)
	}

	cleaner, err := ddebpf.NewMapCleaner[K, V](mapObj, protocols.DefaultMapCleanerBatchSize, mapName, attacherName)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s cleaner: %w", mapName, err)
	}
	return cleaner, nil
}

func (o *sslProgram) initAllMapCleaners() error {
	var err error

	o.sslReadArgsMapCleaner, err = initMapCleaner[uint64, http.SslReadArgs](o.ebpfManager, pidKeyedTLSMaps.SSLReadArgs, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslReadExArgsMapCleaner, err = initMapCleaner[uint64, http.SslReadExArgs](o.ebpfManager, pidKeyedTLSMaps.SSLReadExArgs, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslWriteArgsMapCleaner, err = initMapCleaner[uint64, http.SslWriteArgs](o.ebpfManager, pidKeyedTLSMaps.SSLWriteArgs, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslWriteExArgsMapCleaner, err = initMapCleaner[uint64, http.SslWriteExArgs](o.ebpfManager, pidKeyedTLSMaps.SSLWriteExArgs, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.bioNewSocketArgsMapCleaner, err = initMapCleaner[uint64, uint32](o.ebpfManager, pidKeyedTLSMaps.BioNewSocketArgs, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslCtxByPIDTGIDMapCleaner, err = initMapCleaner[uint64, uint64](o.ebpfManager, pidKeyedTLSMaps.SSLCtxByPIDTGID, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslSockByCtxMapCleaner, err = initMapCleaner[http.SSLCtxPidTGID, http.SslSock](o.ebpfManager, sslSockByCtxMap, UsmTLSAttacherName)
	if err != nil {
		return err
	}

	o.sslCtxByTupleMapCleaner, err = initMapCleaner[http.ConnTuple, uint64](o.ebpfManager, sslCtxByTupleMap, UsmTLSAttacherName)
	if err != nil {
		return err
	}
	return nil
}

// PreStart is called before the start of the provided eBPF manager.
func (o *sslProgram) PreStart() error {
	if err := o.initAllMapCleaners(); err != nil {
		return err
	}
	return o.attacher.Start()
}

// PostStart is a no-op.
func (o *sslProgram) PostStart() error {
	return nil
}

// Stop stops the program.
func (o *sslProgram) Stop() {
	o.attacher.Stop()
}

// DumpMaps dumps the content of the map represented by mapName & currentMap, if it used by the eBPF program, to output.
func (o *sslProgram) DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map) {
	switch mapName {
	case sslSockByCtxMap: // maps/ssl_sock_by_ctx (BPF_MAP_TYPE_HASH), key C.ssl_ctx_pid_tgid_t, value C.ssl_sock_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.ssl_ctx_pid_tgid_t', value: 'C.ssl_sock_t'\n")
		iter := currentMap.Iterate()
		var key http.SSLCtxPidTGID
		var value http.SslSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case sslCtxByTupleMap: // maps/ssl_ctx_by_tuple (BPF_MAP_TYPE_HASH), key C.conn_tuple_t, value C.void *
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.conn_tuple_t', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key http.ConnTuple
		var value uintptr
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.SSLReadArgs: // maps/ssl_read_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_read_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslReadArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.SSLReadExArgs: // maps/ssl_read_ex_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_ex_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_read_ex_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslReadExArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.SSLWriteArgs: // maps/ssl_write_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_write_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_write_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslWriteArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.SSLWriteExArgs: // maps/ssl_write_ex_args_t (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_write_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_write_ex_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslWriteExArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.BioNewSocketArgs: // maps/bio_new_socket_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case fdBySSLBioMap: // maps/fd_by_ssl_bio (BPF_MAP_TYPE_HASH), key C.__u32, value uintptr // C.void *
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u32', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value uintptr // C.void *
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case pidKeyedTLSMaps.SSLCtxByPIDTGID: // maps/ssl_ctx_by_pid_tgid (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.void *
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uintptr // C.void *
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}
	}
}

// IsBuildModeSupported returns always true, as tls module is supported by all modes.
func (*sslProgram) IsBuildModeSupported(buildmode.Type) bool {
	return true
}

// addProcessExitProbe selects the best probe type for intercepting process exits.
// Strategy:
// - Kernel >= 4.17: raw_tracepoint (if HaveProgramType succeeds)
// - Kernel >= 4.15: regular tracepoint (multiple attachments supported)
// - Kernel < 4.15: kprobe on do_exit (fallback for old kernels)
func (o *sslProgram) addProcessExitProbe(options *manager.Options) {
	// Default to kprobe fallback (works on all kernel versions)
	selectedProbe := kprobeDoExit
	excludeProbes := []string{rawTracepointSchedProcessExit, oldTracepointSchedProcessExit}
	var tracepointName string

	kv, err := kernel.HostVersion()
	if err != nil {
		log.Warnf("Failed to get kernel version, using kprobe fallback: %v", err)
	} else {
		kv415 := kernel.VersionCode(4, 15, 0)

		if features.HaveProgramType(ebpf.RawTracepoint) == nil {
			// Raw tracepoint available (kernel >= 4.17 typically)
			selectedProbe = rawTracepointSchedProcessExit
			tracepointName = "sched_process_exit"
			excludeProbes = []string{oldTracepointSchedProcessExit, kprobeDoExit}
			log.Infof("Using raw tracepoint for process exit monitoring (kernel %s supports raw tracepoints)", kv)
		} else if kv >= kv415 {
			// Regular tracepoint with multiple attachment support (kernel >= 4.15)
			selectedProbe = oldTracepointSchedProcessExit
			excludeProbes = []string{rawTracepointSchedProcessExit, kprobeDoExit}
			log.Infof("Using regular tracepoint for process exit monitoring (kernel %s >= 4.15)", kv)
		} else {
			// Kprobe fallback for kernel < 4.15 (no multiple tracepoint attachment)
			log.Infof("Using kprobe fallback for process exit monitoring (kernel %s < 4.15)", kv)
		}
	}

	p := &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			EBPFFuncName: selectedProbe,
			UID:          probeUID,
		},
		TracepointName: tracepointName, // Empty for kprobes, set for tracepoints
	}
	o.ebpfManager.Probes = append(o.ebpfManager.Probes, p)
	options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: p.ProbeIdentificationPair})
	options.ExcludedFunctions = append(options.ExcludedFunctions, excludeProbes...)
}

// pidTgidCleanerCB checks if the pid (upper 32 bits of pidTgid) is in alivePIDs.
func pidTgidCleanerCB(pidTgid uint64, alivePIDs map[uint32]struct{}) bool {
	pid := uint32(pidTgid >> 32)
	_, isAlive := alivePIDs[pid]
	return !isAlive
}

// cleanupDeadPids clears maps of terminated processes, is invoked when raw tracepoints unavailable.
func (o *sslProgram) cleanupDeadPids(alivePIDs map[uint32]struct{}) {
	o.sslReadArgsMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, _ http.SslReadArgs) bool {
		return pidTgidCleanerCB(pidTgid, alivePIDs)
	})

	o.sslReadExArgsMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, _ http.SslReadExArgs) bool {
		return pidTgidCleanerCB(pidTgid, alivePIDs)
	})

	o.sslWriteArgsMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, _ http.SslWriteArgs) bool {
		return pidTgidCleanerCB(pidTgid, alivePIDs)
	})

	o.sslWriteExArgsMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, _ http.SslWriteExArgs) bool {
		return pidTgidCleanerCB(pidTgid, alivePIDs)
	})

	o.bioNewSocketArgsMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, _ uint32) bool {
		return pidTgidCleanerCB(pidTgid, alivePIDs)
	})

	if err := o.deleteDeadPidsInSSLCtxMap(alivePIDs); err != nil {
		log.Debugf("SSL map %q cleanup error: %v", pidKeyedTLSMaps.SSLCtxByPIDTGID, err)
	}
}

// deleteDeadPidsInSSLCtxMap cleans up three related SSL maps in sequence:
// 1. ssl_ctx_by_pid_tgid: Maps PIDs to SSL contexts
// 2. ssl_sock_by_ctx: Maps SSL contexts to socket info
// 3. ssl_ctx_by_tuple: Maps connection tuples to SSL contexts
func (o *sslProgram) deleteDeadPidsInSSLCtxMap(alivePIDs map[uint32]struct{}) error {
	// Track SSL contexts that need to be cleaned up
	// These are contexts belonging to dead PIDs
	sslCtxToClean := make(map[uint64]struct{})

	// First pass: Clean ssl_ctx_by_pid_tgid, ssl_sock_by_ctx map and collect dead SSL contexts
	o.sslCtxByPIDTGIDMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, sslCtx uint64) bool {
		pid := uint32(pidTgid >> 32)
		if _, isAlive := alivePIDs[pid]; !isAlive {
			sslCtxToClean[sslCtx] = struct{}{}
			return true
		}
		return false
	})

	// Second pass: Clean ssl_sock_by_ctx map and add more dead SSL contexts
	o.sslSockByCtxMapCleaner.Clean(nil, nil, func(_ int64, key http.SSLCtxPidTGID, _ http.SslSock) bool {
		pid := uint32(key.Tgid >> 32)
		if _, isAlive := alivePIDs[pid]; !isAlive {
			sslCtxToClean[key.Ctx] = struct{}{}
			return true
		}
		return false
	})

	// Third pass: Clean ssl_ctx_by_tuple map using collected SSL contexts
	o.sslCtxByTupleMapCleaner.Clean(nil, nil, func(_ int64, _ http.ConnTuple, sslCtx uint64) bool {
		_, shouldClean := sslCtxToClean[sslCtx]
		return shouldClean
	})

	return nil
}
