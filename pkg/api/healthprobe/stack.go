// Copyright 2011 The Go Authors. All rights reserved.

package healthprobe

import (
	"runtime"
)

// inspired by https://golang.org/src/runtime/debug/stack.go?s=587:606#L11
func allStack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}
