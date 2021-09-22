// +build linux_bpf

package http

import (
	"os"
	"regexp"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
)

var sslProbes = []string{
	"uprobe/SSL_set_bio",
	"uprobe/SSL_set_fd",
	"uprobe/SSL_read",
	"uretprobe/SSL_read",
	"uprobe/SSL_write",
	"uprobe/SSL_shutdown",
}

var cryptoProbes = []string{
	"uprobe/BIO_new_socket",
	"uretprobe/BIO_new_socket",
}

const (
	sslSockByCtxMap        = "ssl_sock_by_ctx"
	sharedLibrariesPerfMap = "shared_libraries"

	// probe used for streaming shared library events
	doSysOpen    = "kprobe/do_sys_open"
	doSysOpenRet = "kretprobe/do_sys_open"

	// UID used to create the base probes
	baseUID = "base"
)

type openSSLProgram struct {
	cfg         *config.Config
	sockFDMap   *ebpf.Map
	perfHandler *ddebpf.PerfHandler
	watcher     *soWatcher
	manager     *manager.Manager
}

var _ subprogram = &openSSLProgram{}

func newOpenSSLProgram(c *config.Config, sockFDMap *ebpf.Map) (*openSSLProgram, error) {
	if !c.EnableHTTPSMonitoring {
		return nil, nil
	}

	return &openSSLProgram{
		cfg:         c,
		sockFDMap:   sockFDMap,
		perfHandler: ddebpf.NewPerfHandler(batchNotificationsChanSize),
	}, nil
}

func (o *openSSLProgram) ConfigureManager(m *manager.Manager) {
	if o == nil {
		return
	}

	o.manager = m
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
		&manager.Probe{Section: doSysOpen, KProbeMaxActive: maxActive},
		&manager.Probe{Section: doSysOpenRet, KProbeMaxActive: maxActive},
	)

	// Load SSL & Crypto "base" probes
	var extraProbes []string
	extraProbes = append(extraProbes, sslProbes...)
	extraProbes = append(extraProbes, cryptoProbes...)
	for _, sec := range extraProbes {
		m.Probes = append(m.Probes, &manager.Probe{
			Section: sec,
			UID:     baseUID,
		})
	}
}

func (o *openSSLProgram) ConfigureOptions(options *manager.Options) {
	if o == nil {
		return
	}

	options.MapSpecEditors[sslSockByCtxMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(o.cfg.MaxTrackedConnections),
		EditorFlag: manager.EditMaxEntries,
	}

	options.ActivatedProbes = append(options.ActivatedProbes,
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: doSysOpen,
			},
		},
		&manager.ProbeSelector{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: doSysOpenRet,
			},
		},
	)

	if options.MapEditors == nil {
		options.MapEditors = make(map[string]*ebpf.Map)
	}

	options.MapEditors[string(probes.SockByPidFDMap)] = o.sockFDMap
}

func (o *openSSLProgram) Start() {
	if o == nil {
		return
	}

	// Setup shared library watcher and configure the appropriate callbacks
	o.watcher = newSOWatcher(o.cfg.ProcRoot, o.perfHandler,
		soRule{
			re:           regexp.MustCompile(`libssl.so`),
			registerCB:   addHooks(o.manager, sslProbes),
			unregisterCB: removeHooks(o.manager, sslProbes),
		},
		soRule{
			re:           regexp.MustCompile(`libcrypto.so`),
			registerCB:   addHooks(o.manager, cryptoProbes),
			unregisterCB: removeHooks(o.manager, cryptoProbes),
		},
	)

	o.watcher.Start()
}

func (o *openSSLProgram) Stop() {
	if o == nil {
		return
	}

	o.perfHandler.Stop()
}

func addHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := libPath
		for _, sec := range probes {
			p, found := m.GetProbe(manager.ProbeIdentificationPair{uid, sec})
			if found {
				if !p.IsRunning() {
					err := p.Attach()
					if err != nil {
						return err
					}
				}

				continue
			}

			newProbe := manager.Probe{
				Section:    sec,
				BinaryPath: libPath,
				UID:        uid,
			}

			err := m.AddHook(baseUID, newProbe)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func removeHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := libPath
		for _, sec := range probes {
			p, found := m.GetProbe(manager.ProbeIdentificationPair{uid, sec})
			if !found {
				continue
			}
			p.Detach()
		}

		return nil
	}
}
