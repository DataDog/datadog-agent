// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"io"
	"regexp"
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
		Name: sslReadArgsMap,
	},
	{
		Name: sslReadExArgsMap,
	},
	{
		Name: sslWriteArgsMap,
	},
	{
		Name: sslWriteExArgsMap,
	},
	{
		Name: bioNewSocketArgsMap,
	},
	{
		Name: fdBySSLBioMap,
	},
	{
		Name: sslCtxByPIDTGIDMap,
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
	},
}

type sslProgram struct {
	cfg         *config.Config
	attacher    *uprobes.UprobeAttacher
	ebpfManager *manager.Manager

	sslCtxByPIDTGIDMapCleaner *ddebpf.MapCleaner[uint64, uint64]
	sslSockByCtxMapCleaner    *ddebpf.MapCleaner[uint64, http.SslSock]
	sslCtxByTupleMapCleaner   *ddebpf.MapCleaner[http.ConnTuple, uint64]
}

func newSSLProgramProtocolFactory(m *manager.Manager, c *config.Config) (protocols.Protocol, error) {
	if !c.EnableNativeTLSMonitoring || !usmconfig.TLSSupported(c) {
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
	attacherConfig := uprobes.AttacherConfig{
		ProcRoot:                       procRoot,
		Rules:                          rules,
		ExcludeTargets:                 uprobes.ExcludeSelf | uprobes.ExcludeInternal | uprobes.ExcludeBuildkit | uprobes.ExcludeContainerdTmp,
		EbpfConfig:                     &c.Config,
		PerformInitialScan:             true,
		EnablePeriodicScanNewProcesses: true,
		SharedLibsLibsets:              []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
		EnableDetailedLogging:          false,
	}

	o := &sslProgram{
		cfg:         c,
		ebpfManager: m,
	}

	if features.HaveProgramType(ebpf.RawTracepoint) != nil {
		attacherConfig.OnSyncCallback = o.cleanupDeadPids
	}

	var err error
	o.attacher, err = uprobes.NewUprobeAttacher(consts.USMModuleName, UsmTLSAttacherName, attacherConfig, m, uprobes.NopOnAttachCallback, &uprobes.NativeBinaryInspector{}, monitor.GetProcessMonitor())
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
	options.MapSpecEditors[sslCtxByPIDTGIDMap] = manager.MapSpecEditor{
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
func initMapCleaner[K, V interface{}](mgr *manager.Manager, mapName string) (*ddebpf.MapCleaner[K, V], error) {
	mapObj, _, err := mgr.GetMap(mapName)
	if err != nil {
		return nil, fmt.Errorf("dead process ssl cleaner failed to get map: %q error: %w", mapName, err)
	}

	cleaner, err := ddebpf.NewMapCleaner[K, V](mapObj, protocols.DefaultMapCleanerBatchSize, mapName, UsmTLSAttacherName)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s cleaner: %w", mapName, err)
	}
	return cleaner, nil
}

func (o *sslProgram) initAllMapCleaners() error {
	var err error

	o.sslCtxByPIDTGIDMapCleaner, err = initMapCleaner[uint64, uint64](o.ebpfManager, sslCtxByPIDTGIDMap)
	if err != nil {
		return err
	}

	o.sslSockByCtxMapCleaner, err = initMapCleaner[uint64, http.SslSock](o.ebpfManager, sslSockByCtxMap)
	if err != nil {
		return err
	}

	o.sslCtxByTupleMapCleaner, err = initMapCleaner[http.ConnTuple, uint64](o.ebpfManager, sslCtxByTupleMap)
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
	case sslSockByCtxMap: // maps/ssl_sock_by_ctx (BPF_MAP_TYPE_HASH), key uintptr // C.void *, value C.ssl_sock_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'uintptr // C.void *', value: 'C.ssl_sock_t'\n")
		iter := currentMap.Iterate()
		var key uintptr // C.void *
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

	case sslReadArgsMap: // maps/ssl_read_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_read_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslReadArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case sslReadExArgsMap: // maps/ssl_read_ex_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_ex_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_read_ex_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslReadExArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case sslWriteArgsMap: // maps/ssl_write_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_write_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_write_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslWriteArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case sslWriteExArgsMap: // maps/ssl_write_ex_args_t (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_write_args_t
		io.WriteString(w, "Map: '"+mapName+"', key: 'C.__u64', value: 'C.ssl_write_ex_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value http.SslWriteExArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			spew.Fdump(w, key, value)
		}

	case bioNewSocketArgsMap: // maps/bio_new_socket_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
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

	case sslCtxByPIDTGIDMap: // maps/ssl_ctx_by_pid_tgid (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.void *
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

// addProcessExitProbe adds a raw or regular tracepoint program depending on which is supported.
func (o *sslProgram) addProcessExitProbe(options *manager.Options) {
	if features.HaveProgramType(ebpf.RawTracepoint) == nil {
		// use a raw tracepoint on a supported kernel to intercept terminated threads and clear the corresponding maps
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: rawTracepointSchedProcessExit,
				UID:          probeUID,
			},
			TracepointName: "sched_process_exit",
		}
		o.ebpfManager.Probes = append(o.ebpfManager.Probes, p)
		options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: p.ProbeIdentificationPair})
		// exclude regular tracepoint
		options.ExcludedFunctions = append(options.ExcludedFunctions, oldTracepointSchedProcessExit)
	} else {
		// use a regular tracepoint to intercept terminated threads
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: oldTracepointSchedProcessExit,
				UID:          probeUID,
			},
		}
		o.ebpfManager.Probes = append(o.ebpfManager.Probes, p)
		options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: p.ProbeIdentificationPair})
		// exclude a raw tracepoint
		options.ExcludedFunctions = append(options.ExcludedFunctions, rawTracepointSchedProcessExit)
	}
}

var sslPidKeyMaps = []string{
	sslReadArgsMap,
	sslReadExArgsMap,
	sslWriteArgsMap,
	sslWriteExArgsMap,
	bioNewSocketArgsMap,
}

// cleanupDeadPids clears maps of terminated processes, is invoked when raw tracepoints unavailable.
func (o *sslProgram) cleanupDeadPids(alivePIDs map[uint32]struct{}) {
	for _, mapName := range sslPidKeyMaps {
		err := deleteDeadPidsInMap(o.ebpfManager, mapName, alivePIDs)
		if err != nil {
			log.Debugf("SSL map %q cleanup error: %v", mapName, err)
		}
	}
	if err := o.deleteDeadPidsInSSLCtxMap(alivePIDs); err != nil {
		log.Debugf("SSL map %q cleanup error: %v", sslCtxByPIDTGIDMap, err)
	}
}

// deleteDeadPidsInMap finds a map by name and deletes dead processes.
// enters when raw tracepoint is not supported, kernel < 4.17
func deleteDeadPidsInMap(manager *manager.Manager, mapName string, alivePIDs map[uint32]struct{}) error {
	emap, _, err := manager.GetMap(mapName)
	if err != nil {
		return fmt.Errorf("dead process cleaner failed to get map: %q error: %w", mapName, err)
	}

	var keysToDelete []uint64
	var key uint64
	value := make([]byte, emap.ValueSize())
	iter := emap.Iterate()

	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		pid := uint32(key >> 32)
		if _, exists := alivePIDs[pid]; !exists {
			keysToDelete = append(keysToDelete, key)
		}
	}
	for _, k := range keysToDelete {
		_ = emap.Delete(unsafe.Pointer(&k))
	}

	return nil
}

// deleteDeadPidsInSSLCtxMap cleans up three related SSL maps in sequence:
// 1. ssl_ctx_by_pid_tgid: Maps PIDs to SSL contexts
// 2. ssl_sock_by_ctx: Maps SSL contexts to socket info
// 3. ssl_ctx_by_tuple: Maps connection tuples to SSL contexts
func (o *sslProgram) deleteDeadPidsInSSLCtxMap(alivePIDs map[uint32]struct{}) error {
	// Track SSL contexts that need to be cleaned up
	// These are contexts belonging to dead PIDs
	sslCtxToClean := make(map[uint64]struct{})

	// First pass: Clean ssl_ctx_by_pid_tgid map and collect dead SSL contexts
	o.sslCtxByPIDTGIDMapCleaner.Clean(nil, nil, func(_ int64, pidTgid uint64, sslCtx uint64) bool {
		pid := uint32(pidTgid >> 32)
		if _, isAlive := alivePIDs[pid]; !isAlive {
			sslCtxToClean[sslCtx] = struct{}{}
			return true
		}
		return false
	})

	// Second pass: Clean ssl_sock_by_ctx map using collected SSL contexts
	o.sslSockByCtxMapCleaner.Clean(nil, nil, func(_ int64, sslCtx uint64, _ http.SslSock) bool {
		_, shouldClean := sslCtxToClean[sslCtx]
		return shouldClean
	})

	// Third pass: Clean ssl_ctx_by_tuple map using collected SSL contexts
	o.sslCtxByTupleMapCleaner.Clean(nil, nil, func(_ int64, _ http.ConnTuple, sslCtx uint64) bool {
		_, shouldClean := sslCtxToClean[sslCtx]
		return shouldClean
	})

	return nil
}
