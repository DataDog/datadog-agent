// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is used for testing purposes and it essentially maps into memory
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/exp/mmap"
)

// This program is used for testing purposes and it essentially maps into memory
// one or more files provided as command line arguments
func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	readers := make([]*mmap.ReaderAt, 0, len(os.Args)-1)
	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}()
	for _, path := range os.Args[1:] {
		r, err := mmap.Open(path)
		if err != nil {
			panic(err)
		}
		readers = append(readers, r)
	}

	go func() {
		<-sigs
		done <- true
	}()

	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}
