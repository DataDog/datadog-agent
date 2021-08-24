// +build linux_bpf

package http

import (
	"strconv"
	"strings"

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

	sharedLibraries := findOpenSSLLibraries(c.ProcRoot)
	for i, lib := range sharedLibraries {
		probes := sslProbes
		if strings.Contains(lib.HostPath, "crypto") {
			probes = cryptoProbes
		}

		addHooks(m, probes, lib.HostPath, i)
	}
}

func addHooks(m *manager.Manager, probes []string, libPath string, i int) {
	uid := strconv.Itoa(i)
	for _, sec := range probes {
		newProbe := manager.Probe{
			Section:    sec,
			BinaryPath: libPath,
			UID:        uid,
		}

		err := m.AddHook(baseUID, newProbe)
		if err != nil {
			log.Errorf("error cloning %s: %s", sec, err)
		}
	}
}
