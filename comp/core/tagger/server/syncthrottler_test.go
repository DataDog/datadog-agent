package server

import (
	"fmt"
	"testing"
	"time"
)

func TestSyncThrottler(t *testing.T) {

	throtler := NewSyncThrottler(3)

	t1 := throtler.RequestToken()
	t2 := throtler.RequestToken()
	t3 := throtler.RequestToken()
	go func() {
		time.Sleep(5 * time.Second)
		throtler.Release(t3)
		fmt.Println("t3 is released")
	}()
	t4 := throtler.RequestToken()
	throtler.Release(t4)
	throtler.Release(t1)
	throtler.Release(t2)
}
