package common

// MinUint64 returns the min of the two passed number
func MinUint64(a uint64, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// MaxUint64 returns the max of the two passed number
func MaxUint64(a uint64, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
