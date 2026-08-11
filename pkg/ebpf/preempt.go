package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func EBPFPreemptCountSupported() bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("preempt count: could not determine the current kernel version.")
		return false
	}

	return kversion >= kernel.VersionCode(5, 10, 0)
}

func PreemptCountConstants() (map[string]uint64, error) {
	preemptCountMissing, err := VerifyKernelFuncs("__preempt_count")
	if err != nil {
		return nil, fmt.Errorf("error verifying kernel symbol: %w", err)
	}

	if len(preemptCountMissing) == 0 {
		return map[string]uint64{"use_preempt_count": uint64(1)}, nil
	}

	return map[string]uint64{}, nil
}
