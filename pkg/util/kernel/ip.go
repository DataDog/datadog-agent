// +build linux

package kernel

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// IsIPv6Enabled returns whether or not IPv6 has been enabled on the host
func IsIPv6Enabled() bool {
	ints, err := ioutil.ReadFile(filepath.Join(util.GetProcRoot(), "net/if_inet6"))
	return err == nil && strings.TrimSpace(string(ints)) != ""
}
