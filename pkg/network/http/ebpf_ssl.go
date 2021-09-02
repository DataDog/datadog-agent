// +build linux_bpf

package http

import (
	"regexp"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
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

func initSSLTracing(c *config.Config, m *manager.Manager, perfHandler *ddebpf.PerfHandler) {
	if !c.EnableHTTPSMonitoring {
		return
	}

	watcher := newSOWatcher(c.ProcRoot, perfHandler,
		soRule{
			re:           regexp.MustCompile(`libssl.so`),
			registerCB:   addHooks(m, sslProbes),
			unregisterCB: removeHooks(m, sslProbes),
		},
		soRule{
			re:           regexp.MustCompile(`libcrypto.so`),
			registerCB:   addHooks(m, cryptoProbes),
			unregisterCB: removeHooks(m, cryptoProbes),
		},
	)

	watcher.Start()
}

func addHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := generateUID(libPath)
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
		uid := generateUID(libPath)
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

func generateUID(s string) string {
	return s
}
