// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	fds := make([]*os.File, len(os.Args)-1)
	defer func() {
		for _, fd := range fds {
			_ = fd.Close()
		}
	}()
	for _, path := range os.Args[1:] {
		fd, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		fds = append(fds, fd)
	}

	go func() {
		<-sigs
		done <- true
	}()

	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}
