// +build linux_bpf

package http

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func initSSLTracing(m *manager.Manager, c *config.Config) {
	if !c.EnableHTTPSMonitoring {
		return
	}

	watcher := newSOWatcher(c.ProcRoot,
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
		activated := make([]manager.Probe, 0, len(probes))
		uid := generateUID(libPath)
		for _, sec := range probes {
			newProbe := manager.Probe{
				Section:    sec,
				BinaryPath: libPath,
				UID:        uid,
			}

			err := m.AddHook(baseUID, newProbe)
			if err == nil {
				activated = append(activated, newProbe)
				continue
			}

			// If we had an error halfway through we detach the previous probes that were attached
			for _, p := range activated {
				m.DetachHook(p.Section, p.UID)
			}
			return err
		}

		log.Debugf("https: attached probes for %s", libPath)
		return nil
	}
}

func removeHooks(m *manager.Manager, probes []string) func(string) error {
	return func(libPath string) error {
		uid := generateUID(libPath)
		for _, sec := range probes {
			m.DetachHook(sec, uid)
		}

		log.Debugf("https: detached probes for %s", libPath)
		return nil
	}
}

func generateUID(s string) string {
	return s
}
