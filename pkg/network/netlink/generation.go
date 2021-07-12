package netlink

import (
	"time"
)

const (
	maxUint8 uint8 = ^(uint8(0))
)

// getCurrentGeneration returns the "generation" of the current timestamp. A compact representation
// of the current time. Every generation is a discrete interval of genLength duration, and generation numbers
// roll over. So the 0th and 256th generation are both represented as 0.
func getCurrentGeneration(genLength time.Duration, nowNanos int64) uint8 {
	genLengthNanos := genLength.Nanoseconds()
	return uint8((nowNanos / genLengthNanos) % int64(maxUint8))
}

func getNthGeneration(genLength time.Duration, nowNanos int64, n uint8) uint8 {
	curr := getCurrentGeneration(genLength, nowNanos)
	return (curr + n) % maxUint8
}
