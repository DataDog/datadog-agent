package checks

import "time"

func calculateCtrPct(cur, prev float64, sys2, sys1 uint64, numCPU int, before time.Time) float32 {
	// -1 is returned if a cgroup file is missing or the `ContainerCPUStats` object is nil.
	// In these situations, return -1 so that the metric is skipped on the backend.
	if cur == -1 || prev == -1 {
		return -1
	}
	now := time.Now()
	diff := now.Unix() - before.Unix()
	if before.IsZero() || diff <= 0 {
		return 0
	}

	// Prevent uint underflows
	if prev > cur || sys1 > sys2 {
		return 0
	}

	// If we have system usage values then we need to calculate against those.
	// XXX: Right now this only applies to ECS collection. Note that the inclusion of CPUs is
	// necessary because the value gets normalized against the CPU limit, which also accounts for CPUs.
	if sys1 >= 0 && sys2 > 0 && sys2 != sys1 {
		cpuDelta := float32(cur - prev)
		sysDelta := float32(sys2 - sys1)
		return (cpuDelta / sysDelta) * float32(numCPU) * 100
	}
	return float32(cur-prev) / float32(diff)
}
