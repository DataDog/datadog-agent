// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"debug/elf"
	"fmt"
	"regexp"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var openSSLProbes = []manager.ProbesSelector{
	&manager.BestEffort{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_read_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_read_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_write_ex",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_write_ex",
				},
			},
		},
	},
	&manager.AllOf{
		Selectors: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_do_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_do_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_connect",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_connect",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_set_bio",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_set_fd",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_read",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__SSL_write",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__SSL_write",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
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
					EBPFFuncName: "uprobe__BIO_new_socket",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
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
					EBPFFuncName: "uprobe__gnutls_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__gnutls_handshake",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_transport_set_int2",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_transport_set_ptr",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_transport_set_ptr2",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_record_recv",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__gnutls_record_recv",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_record_send",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__gnutls_record_send",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uprobe__gnutls_bye",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
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

type sslProgram struct {
	cfg       *config.Config
	sockFDMap *ebpf.Map
	manager   *errtelemetry.Manager
	watcher   *sharedlibraries.Watcher
}

var _ subprogram = &sslProgram{}

func newSSLProgram(c *config.Config, m *manager.Manager, sockFDMap *ebpf.Map, bpfTelemetry *errtelemetry.EBPFTelemetry) *sslProgram {
	if !c.EnableHTTPSMonitoring || !http.HTTPSSupported(c) {
		return nil
	}

	watcher, err := sharedlibraries.NewWatcher(c, bpfTelemetry,
		sharedlibraries.Rule{
			Re:           regexp.MustCompile(`libssl.so`),
			RegisterCB:   addHooks(m, openSSLProbes),
			UnregisterCB: removeHooks(m, openSSLProbes),
		},
		sharedlibraries.Rule{
			Re:           regexp.MustCompile(`libcrypto.so`),
			RegisterCB:   addHooks(m, cryptoProbes),
			UnregisterCB: removeHooks(m, cryptoProbes),
		},
		sharedlibraries.Rule{
			Re:           regexp.MustCompile(`libgnutls.so`),
			RegisterCB:   addHooks(m, gnuTLSProbes),
			UnregisterCB: removeHooks(m, gnuTLSProbes),
		},
	)
	if err != nil {
		log.Errorf("error initializating shared library watcher: %s", err)
		return nil
	}

	return &sslProgram{
		cfg:       c,
		sockFDMap: sockFDMap,
		watcher:   watcher,
	}
}

func (o *sslProgram) Name() string {
	return "openssl"
}

func (o *sslProgram) IsBuildModeSupported(_ buildMode) bool {
	return true
}

func (o *sslProgram) ConfigureManager(m *errtelemetry.Manager) {
	o.manager = m
}

func (o *sslProgram) ConfigureOptions(options *manager.Options) {
	options.MapSpecEditors[sslSockByCtxMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: o.cfg.MaxTrackedConnections,
		EditorFlag: manager.EditMaxEntries,
	}

	if options.MapEditors == nil {
		options.MapEditors = make(map[string]*ebpf.Map)
	}

	options.MapEditors[probes.SockByPidFDMap] = o.sockFDMap
}

func (o *sslProgram) Start() {
	o.watcher.Start()
}

func (o *sslProgram) Stop() {
	o.watcher.Stop()
}

func addHooks(m *manager.Manager, probes []manager.ProbesSelector) func(utils.FilePath) error {
	return func(fpath utils.FilePath) error {
		uid := getUID(fpath.ID)

		elfFile, err := elf.Open(fpath.HostPath)
		if err != nil {
			return err
		}
		defer elfFile.Close()

		symbolsSet := make(common.StringSet, 0)
		symbolsSetBestEffort := make(common.StringSet, 0)
		for _, singleProbe := range probes {
			_, isBestEffort := singleProbe.(*manager.BestEffort)
			for _, selector := range singleProbe.GetProbesIdentificationPairList() {
				_, symbol, ok := strings.Cut(selector.EBPFFuncName, "__")
				if !ok {
					continue
				}
				if isBestEffort {
					symbolsSetBestEffort[symbol] = struct{}{}
				} else {
					symbolsSet[symbol] = struct{}{}
				}
			}
		}
		symbolMap, err := bininspect.GetAllSymbolsByName(elfFile, symbolsSet)
		if err != nil {
			return err
		}
		/* Best effort to resolve symbols, so we don't care about the error */
		symbolMapBestEffort, _ := bininspect.GetAllSymbolsByName(elfFile, symbolsSetBestEffort)

		for _, singleProbe := range probes {
			_, isBestEffort := singleProbe.(*manager.BestEffort)
			for _, selector := range singleProbe.GetProbesIdentificationPairList() {
				identifier := manager.ProbeIdentificationPair{
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

				_, symbol, ok := strings.Cut(selector.EBPFFuncName, "__")
				if !ok {
					continue
				}

				sym := symbolMap[symbol]
				if isBestEffort {
					sym, found = symbolMapBestEffort[symbol]
					if !found {
						continue
					}
				}
				manager.SanitizeUprobeAddresses(elfFile, []elf.Symbol{sym})
				offset, err := bininspect.SymbolToOffset(elfFile, sym)
				if err != nil {
					return err
				}

				newProbe := &manager.Probe{
					ProbeIdentificationPair: identifier,
					BinaryPath:              fpath.HostPath,
					UprobeOffset:            uint64(offset),
					HookFuncName:            symbol,
				}
				if err := m.AddHook("", newProbe); err == nil {
					ebpfcheck.AddProgramNameMapping(newProbe.ID(), fmt.Sprintf("%s_%s", newProbe.EBPFFuncName, identifier.UID), "usm_tls")
				}
			}
			if err := singleProbe.RunValidator(m); err != nil {
				return err
			}
		}

		return nil
	}
}

func removeHooks(m *manager.Manager, probes []manager.ProbesSelector) func(utils.FilePath) error {
	return func(fpath utils.FilePath) error {
		uid := getUID(fpath.ID)
		for _, singleProbe := range probes {
			for _, selector := range singleProbe.GetProbesIdentificationPairList() {
				identifier := manager.ProbeIdentificationPair{
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
					log.Debugf("detach hook %s/%s : %s", selector.EBPFFuncName, uid, err)
				}
				if program != nil {
					program.Close()
				}
			}
		}

		return nil
	}
}

// getUID() return a key of length 5 as the kernel uprobe registration path is limited to a length of 64
// ebpf-manager/utils.go:GenerateEventName() MaxEventNameLen = 64
// MAX_EVENT_NAME_LEN (linux/kernel/trace/trace.h)
//
// Length 5 is arbitrary value as the full string of the eventName format is
//
//	fmt.Sprintf("%s_%.*s_%s_%s", probeType, maxFuncNameLen, functionName, UID, attachPIDstr)
//
// functionName is variable but with a minimum guarantee of 10 chars
func getUID(lib utils.PathIdentifier) string {
	return lib.Key()[:5]
}

func (*sslProgram) GetAllUndefinedProbes() []manager.ProbeIdentificationPair {
	var probeList []manager.ProbeIdentificationPair

	for _, sslProbeList := range [][]manager.ProbesSelector{openSSLProbes, cryptoProbes, gnuTLSProbes} {
		for _, singleProbe := range sslProbeList {
			for _, identifier := range singleProbe.GetProbesIdentificationPairList() {
				probeList = append(probeList, manager.ProbeIdentificationPair{
					EBPFFuncName: identifier.EBPFFuncName,
				})
			}
		}
	}

	return probeList
}
