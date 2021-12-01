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
	"runtime"
	"strconv"
	"strings"

	"github.com/twmb/murmur3"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

var openSSLProbes = map[string]string{
	"uprobe/SSL_set_bio":  "uprobe__SSL_set_bio",
	"uprobe/SSL_set_fd":   "uprobe__SSL_set_fd",
	"uprobe/SSL_read":     "uprobe__SSL_read",
	"uretprobe/SSL_read":  "uretprobe__SSL_read",
	"uprobe/SSL_write":    "uprobe__SSL_write",
	"uprobe/SSL_shutdown": "uprobe__SSL_shutdown",
}

var cryptoProbes = map[string]string{
	"uprobe/BIO_new_socket":    "uprobe__BIO_new_socket",
	"uretprobe/BIO_new_socket": "uretprobe__BIO_new_socket",
}

var gnuTLSProbes = map[string]string{
	"uprobe/gnutls_transport_set_int2": "uprobe__gnutls_transport_set_int2",
	"uprobe/gnutls_transport_set_ptr":  "uprobe__gnutls_transport_set_ptr",
	"uprobe/gnutls_transport_set_ptr2": "uprobe__gnutls_transport_set_ptr2",
	"uprobe/gnutls_record_recv":        "uprobe__gnutls_record_recv",
	"uretprobe/gnutls_record_recv":     "uretprobe__gnutls_record_recv",
	"uprobe/gnutls_record_send":        "uprobe__gnutls_record_send",
	"uprobe/gnutls_bye":                "uprobe__gnutls_bye",
	"uprobe/gnutls_deinit":             "uprobe__gnutls_deinit",
}

const (
	sslSockByCtxMap        = "ssl_sock_by_ctx"
	sharedLibrariesPerfMap = "shared_libraries"

	// probe used for streaming shared library events
	doSysOpen    = "kprobe/do_sys_open"
	doSysOpenRet = "kretprobe/do_sys_open"
)

type sslProgram struct {
	cfg         *config.Config
	sockFDMap   *ebpf.Map
	perfHandler *ddebpf.PerfHandler
	watcher     *soWatcher
	manager     *manager.Manager
}

var _ subprogram = &sslProgram{}

func newSSLProgram(c *config.Config, sockFDMap *ebpf.Map) (*sslProgram, error) {
	if !c.EnableHTTPSMonitoring {
		return nil, nil
	}

	return &sslProgram{
		cfg:         c,
		sockFDMap:   sockFDMap,
		perfHandler: ddebpf.NewPerfHandler(batchNotificationsChanSize),
	}, nil
}

func (o *sslProgram) ConfigureManager(m *manager.Manager) {
	if o == nil {
		return
	}

	o.manager = m

	if !runningOnARM() {
		m.PerfMaps = append(m.PerfMaps, &manager.PerfMap{
			Map: manager.Map{Name: sharedLibrariesPerfMap},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: 8 * os.Getpagesize(),
				Watermark:          1,
				DataHandler:        o.perfHandler.DataHandler,
				LostHandler:        o.perfHandler.LostHandler,
			},
		})

		m.Probes = append(m.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  doSysOpen,
				EBPFFuncName: "kprobe__do_sys_open",
			}, KProbeMaxActive: maxActive},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  doSysOpenRet,
				EBPFFuncName: "kretprobe__do_sys_open",
			}, KProbeMaxActive: maxActive},
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

	if !runningOnARM() {
		options.ActivatedProbes = append(options.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  doSysOpen,
					EBPFFuncName: "kprobe__do_sys_open",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  doSysOpenRet,
					EBPFFuncName: "kretprobe__do_sys_open",
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

func addHooks(m *manager.Manager, probes map[string]string) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)
		for sec, funcName := range probes {
			p, found := m.GetProbe(manager.ProbeIdentificationPair{
				EBPFSection:  sec,
				EBPFFuncName: funcName,
				UID:          uid,
			})
			if found {
				if !p.IsRunning() {
					err := p.Attach()
					if err != nil {
						return err
					}
				}

				continue
			}

			newProbe := &manager.Probe{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  sec,
					EBPFFuncName: funcName,
					UID:          uid,
				},
				BinaryPath: libPath,
			}

			err := m.AddHook("", newProbe)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func removeHooks(m *manager.Manager, probes map[string]string) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)
		for sec, funcName := range probes {
			p, found := m.GetProbe(manager.ProbeIdentificationPair{
				EBPFSection:  sec,
				EBPFFuncName: funcName,
				UID:          uid,
			})
			if !found {
				continue
			}

			program := p.Program()
			m.DetachHook(manager.ProbeIdentificationPair{
				EBPFSection:  sec,
				EBPFFuncName: funcName,
				UID:          uid,
			})
			if program != nil {
				program.Close()
			}
		}

		return nil
	}
}

func runningOnARM() bool {
	return strings.HasPrefix(runtime.GOARCH, "arm")
}

func getUID(libPath string) string {
	sum := murmur3.StringSum64(libPath)
	hash := strconv.FormatInt(int64(sum), 16)
	if len(hash) >= 5 {
		return hash[len(hash)-5:]
	}

	return libPath
}
