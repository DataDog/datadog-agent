// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command fault is a basic go program to be used with dyninst tests.
// It is used to exercise the functionality of the bpf program when a failure to
// read memory occurs.
package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"syscall"
	"unsafe"
)

func main() {
	debug.SetGCPercent(-1) // don't let GC go fault the mmap string
	_, err := fmt.Scanln()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	f, err := os.CreateTemp("", "fault-test")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	os.Remove(f.Name())
	if _, err = f.WriteString("test"); err != nil {
		log.Fatalf("error: %v", err)
	}
	if err = f.Sync(); err != nil {
		log.Fatalf("error: %v", err)
	}
	v, err := syscall.Mmap(int(f.Fd()), 0, 4, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("error: %v", err)
	}
	s := unsafe.String(unsafe.SliceData(v), 4)
	var i int
	callMe("a", s, &i)
	if err := syscall.Munmap(v); err != nil {
		log.Fatalf("error: %v", err)
	}
}

//go:noinline
func callMe(s1, s2 string, i *int) {
	fmt.Println(s1, s2, i)
}
