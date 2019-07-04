package testcommon

import (
	"fmt"
	"os"
	"testing"
	"unsafe"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	ret := m.Run()
	os.Exit(ret)
}

func TestCgoFree(t *testing.T) {
	callCgoFree(nil)
	if cgoFreeCalled != false {
		t.Errorf("freeing NULL should not haved called the cgoFree callback")
	}

	v := 21
	callCgoFree(unsafe.Pointer(&v))
	if cgoFreeCalled != true {
		t.Errorf("freeing a pointer should have called the cgoFree callback")
	}
	if unsafe.Pointer(&v) != latestFreePtr {
		t.Errorf("Freed pointer was not the same as the one given to the callback")
	}
}
