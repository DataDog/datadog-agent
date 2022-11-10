// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"os"
	"regexp"
	"strconv"

	"github.com/twmb/murmur3"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var openSSLProbes = []manager.ProbesSelector{
	&manager.BestEffort{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_read_ex",
					EBPFFuncName: "uprobe__SSL_read_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_read_ex",
					EBPFFuncName: "uretprobe__SSL_read_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_write_ex",
					EBPFFuncName: "uprobe__SSL_write_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_write_ex",
					EBPFFuncName: "uretprobe__SSL_write_ex",
				},
			},
		},
	},
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_do_handshake",
					EBPFFuncName: "uprobe__SSL_do_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_do_handshake",
					EBPFFuncName: "uretprobe__SSL_do_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_connect",
					EBPFFuncName: "uprobe__SSL_connect",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_connect",
					EBPFFuncName: "uretprobe__SSL_connect",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_set_bio",
					EBPFFuncName: "uprobe__SSL_set_bio",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_set_fd",
					EBPFFuncName: "uprobe__SSL_set_fd",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_read",
					EBPFFuncName: "uprobe__SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_read",
					EBPFFuncName: "uretprobe__SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_write",
					EBPFFuncName: "uprobe__SSL_write",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/SSL_write",
					EBPFFuncName: "uretprobe__SSL_write",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/SSL_shutdown",
					EBPFFuncName: "uprobe__SSL_shutdown",
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
					EBPFSection:  "uprobe/BIO_new_socket",
					EBPFFuncName: "uprobe__BIO_new_socket",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/BIO_new_socket",
					EBPFFuncName: "uretprobe__BIO_new_socket",
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
					EBPFSection:  "uprobe/gnutls_handshake",
					EBPFFuncName: "uprobe__gnutls_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/gnutls_handshake",
					EBPFFuncName: "uretprobe__gnutls_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_transport_set_int2",
					EBPFFuncName: "uprobe__gnutls_transport_set_int2",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_transport_set_ptr",
					EBPFFuncName: "uprobe__gnutls_transport_set_ptr",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_transport_set_ptr2",
					EBPFFuncName: "uprobe__gnutls_transport_set_ptr2",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_record_recv",
					EBPFFuncName: "uprobe__gnutls_record_recv",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/gnutls_record_recv",
					EBPFFuncName: "uretprobe__gnutls_record_recv",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_record_send",
					EBPFFuncName: "uprobe__gnutls_record_send",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uretprobe/gnutls_record_send",
					EBPFFuncName: "uretprobe__gnutls_record_send",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_bye",
					EBPFFuncName: "uprobe__gnutls_bye",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "uprobe/gnutls_deinit",
					EBPFFuncName: "uprobe__gnutls_deinit",
				},
			},
		},
	},
}

const (
	sslSockByCtxMap        = "ssl_sock_by_ctx"
	sharedLibrariesPerfMap = "shared_libraries"
)

type ebpfSectionFunction struct {
	section  string
	function string
}

// probe used for streaming shared library events
var (
	kprobeKretprobePrefix = []string{"kprobe", "kretprobe"}
	doSysOpen             = ebpfSectionFunction{section: "do_sys_open", function: "do_sys_open"}
	doSysOpenAt2          = ebpfSectionFunction{section: "do_sys_openat2", function: "do_sys_openat2"}
)

type sslProgram struct {
	cfg         *config.Config
	sockFDMap   *ebpf.Map
	perfHandler *ddebpf.PerfHandler
	watcher     *soWatcher
	manager     *errtelemetry.Manager
}

var _ subprogram = &sslProgram{}

func newSSLProgram(c *config.Config, sockFDMap *ebpf.Map) *sslProgram {
	if !c.EnableHTTPSMonitoring || !HTTPSSupported(c) {
		return nil
	}

	return &sslProgram{
		cfg:         c,
		sockFDMap:   sockFDMap,
		perfHandler: ddebpf.NewPerfHandler(batchNotificationsChanSize),
	}
}

func (o *sslProgram) ConfigureManager(m *errtelemetry.Manager) {
	if o == nil {
		return
	}

	o.manager = m

	m.PerfMaps = append(m.PerfMaps, &manager.PerfMap{
		Map: manager.Map{Name: sharedLibrariesPerfMap},
		PerfMapOptions: manager.PerfMapOptions{
			PerfRingBufferSize: 8 * os.Getpagesize(),
			Watermark:          1,
			RecordHandler:      o.perfHandler.RecordHandler,
			LostHandler:        o.perfHandler.LostHandler,
			RecordGetter:       o.perfHandler.RecordGetter,
		},
	})

	probeSysOpen := doSysOpen
	if sysOpenAt2Supported(o.cfg) {
		probeSysOpen = doSysOpenAt2
	}

	for _, kprobe := range kprobeKretprobePrefix {
		m.Probes = append(m.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  kprobe + "/" + probeSysOpen.section,
				EBPFFuncName: kprobe + "__" + probeSysOpen.function,
				UID:          probeUID,
			},
				KProbeMaxActive: maxActive,
			},
		)
	}
}

func (o *sslProgram) ConfigureOptions(options *manager.Options) {
	if o == nil {
		return
	}

	options.MapSpecEditors[sslSockByCtxMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(o.cfg.MaxTrackedConnections),
		EditorFlag: manager.EditMaxEntries,
	}

	probeSysOpen := doSysOpen
	if sysOpenAt2Supported(o.cfg) {
		probeSysOpen = doSysOpenAt2
	}
	for _, kprobe := range kprobeKretprobePrefix {
		options.ActivatedProbes = append(options.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  kprobe + "/" + probeSysOpen.section,
					EBPFFuncName: kprobe + "__" + probeSysOpen.function,
					UID:          probeUID,
				},
			},
		)
	}

	if options.MapEditors == nil {
		options.MapEditors = make(map[string]*ebpf.Map)
	}

	options.MapEditors[string(probes.SockByPidFDMap)] = o.sockFDMap
}

func (o *sslProgram) Start() {
	if o == nil {
		return
	}

	// Setup shared library watcher and configure the appropriate callbacks
	o.watcher = newSOWatcher(o.cfg.ProcRoot, o.perfHandler,
		soRule{
			re:           regexp.MustCompile(`libssl.so`),
			registerCB:   addHooks(o.manager, openSSLProbes),
			unregisterCB: removeHooks(o.manager, openSSLProbes),
		},
		soRule{
			re:           regexp.MustCompile(`libcrypto.so`),
			registerCB:   addHooks(o.manager, cryptoProbes),
			unregisterCB: removeHooks(o.manager, cryptoProbes),
		},
		soRule{
			re:           regexp.MustCompile(`libgnutls.so`),
			registerCB:   addHooks(o.manager, gnuTLSProbes),
			unregisterCB: removeHooks(o.manager, gnuTLSProbes),
		},
	)

	o.watcher.Start()
}

func (o *sslProgram) Stop() {
	if o == nil {
		return
	}

	o.perfHandler.Stop()
}

func addHooks(m *errtelemetry.Manager, probes []manager.ProbesSelector) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)

		for _, singleProbe := range probes {
			for _, selector := range singleProbe.GetProbesIdentificationPairList() {
				identifier := manager.ProbeIdentificationPair{
					EBPFSection:  selector.EBPFSection,
					EBPFFuncName: selector.EBPFFuncName,
					UID:          uid,
				}
				singleProbe.EditProbeIdentificationPair(selector, identifier)
				probe, found := m.GetProbe(identifier)
				if found {
					if !probe.IsRunning() {
						err := probe.Attach()
						if err != nil {
							return err
						}
					}

					continue
				}

				newProbe := &manager.Probe{
					ProbeIdentificationPair: identifier,
					BinaryPath:              libPath,
				}
				_ = m.AddHook("", newProbe)
			}
			if err := singleProbe.RunValidator(m.Manager); err != nil {
				return err
			}
		}

		return nil
	}
}

func removeHooks(m *errtelemetry.Manager, probes []manager.ProbesSelector) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)
		for _, singleProbe := range probes {
			for _, selector := range singleProbe.GetProbesIdentificationPairList() {
				identifier := manager.ProbeIdentificationPair{
					EBPFSection:  selector.EBPFSection,
					EBPFFuncName: selector.EBPFFuncName,
					UID:          uid,
				}
				probe, found := m.GetProbe(identifier)
				if !found {
					continue
				}

				program := probe.Program()
				err := m.DetachHook(identifier)
				if err != nil {
					log.Debugf("detach hook %s/%s/%s : %s", selector.EBPFSection, selector.EBPFFuncName, uid, err)
				}
				if program != nil {
					program.Close()
				}
			}
		}

		return nil
	}
}

func getUID(libPath string) string {
	sum := murmur3.StringSum64(libPath)
	hash := strconv.FormatInt(int64(sum), 16)
	if len(hash) >= 5 {
		return hash[len(hash)-5:]
	}

	return libPath
}

func (o *sslProgram) GetAllUndefinedProbes() []manager.ProbeIdentificationPair {
	var probeList []manager.ProbeIdentificationPair

	for _, sslProbeList := range [][]manager.ProbesSelector{openSSLProbes, cryptoProbes, gnuTLSProbes} {
		for _, singleProbe := range sslProbeList {
			for _, identifier := range singleProbe.GetProbesIdentificationPairList() {
				probeList = append(probeList, manager.ProbeIdentificationPair{
					EBPFSection:  identifier.EBPFSection,
					EBPFFuncName: identifier.EBPFFuncName,
				})
			}
		}
	}

	for _, hook := range []ebpfSectionFunction{doSysOpen, doSysOpenAt2} {
		for _, kprobe := range kprobeKretprobePrefix {
			probeList = append(probeList, manager.ProbeIdentificationPair{
				EBPFSection:  kprobe + "/" + hook.section,
				EBPFFuncName: kprobe + "__" + hook.function,
			})
		}
	}

	return probeList
}
