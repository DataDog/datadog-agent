package checks

import "time"

func calculateCtrPct(cur, prev, sys2, sys1 uint64, numCPU int, before time.Time) float32 {
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
	// XXX: Right now this only applies to ECS collection
	if sys1 > 0 && sys2 > 0 && sys2 != sys1 {
		cpuDelta := float32(cur - prev)
		sysDelta := float32(sys2 - sys1)
		return (cpuDelta / sysDelta) * float32(numCPU) * 100
	}
	return float32(cur-prev) / float32(diff)
}
