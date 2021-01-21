package kernel

import (
	"io/ioutil"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// IsIPv6Enabled returns whether or not IPv6 has been enabled on the host
func IsIPv6Enabled() bool {
	_, err := ioutil.ReadFile(filepath.Join(util.GetProcRoot(), "net/if_inet6"))
	return err == nil
}
