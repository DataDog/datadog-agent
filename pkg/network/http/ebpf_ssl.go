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
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
)

var openSSLProbes = []string{
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

var gnuTLSProbes = []string{
	"uprobe/gnutls_transport_set_int2",
	"uprobe/gnutls_transport_set_ptr",
	"uprobe/gnutls_transport_set_ptr2",
	"uprobe/gnutls_record_recv",
	"uretprobe/gnutls_record_recv",
	"uprobe/gnutls_record_send",
	"uprobe/gnutls_bye",
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
			&manager.Probe{Section: doSysOpen, KProbeMaxActive: maxActive},
			&manager.Probe{Section: doSysOpenRet, KProbeMaxActive: maxActive},
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
					Section: doSysOpen,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: doSysOpenRet,
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

func addHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)
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

			err := m.AddHook("", newProbe)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func removeHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := getUID(libPath)
		for _, sec := range probes {
			p, found := m.GetProbe(manager.ProbeIdentificationPair{uid, sec})
			if !found {
				continue
			}

			program := p.Program()
			m.DetachHook(sec, uid)
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
