// +build !linux_bpf

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func getRuntimeCompiledProbe(config *config.Config, useSyscallWrapper bool) (bytecode.AssetReader, error) {
	return nil, fmt.Errorf("runtime compilation unsupported")
}
