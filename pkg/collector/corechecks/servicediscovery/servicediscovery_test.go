package servicediscovery

import (
	"testing"
	"time"
)

func TestTimer(t *testing.T) {
	var myTimer timer = realTime{}
	val := myTimer.Now()
	compare := time.Now()
	// should be basically the same time, within a millisecond
	if compare.Sub(val).Truncate(time.Millisecond) != 0 {
		t.Errorf("expected within a millisecond: %v, %v", compare, val)
	}
}
