// +build windows,npm

package tracer

import "testing"

func dnsSupported(t *testing.T) bool {
	return true
}

func httpSupported(t *testing.T) bool {
	return false
}

func connectionBufferCapacity(t *Tracer) int {
	return t.connStatsActive.Capacity()
}
