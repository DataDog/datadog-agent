// +build python,test

package python

import "testing"

func TestHealthCheckData(t *testing.T) {
	testHealthCheckData(t)
}

func TestHealthStartSnapshot(t *testing.T) {
	testHealthStartSnapshot(t)
}

func TestHealthStopSnapshot(t *testing.T) {
	testHealthStopSnapshot(t)
}

func TestNoSubStream(t *testing.T) {
	testNoSubStream(t)
}
