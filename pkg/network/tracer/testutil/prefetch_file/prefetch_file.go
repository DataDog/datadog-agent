// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a simple utility to prefetch files into the page cache.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: prefetch_file <filename>....")
		os.Exit(1)
	}

	closers := make([]func() error, 0, len(os.Args)-1)
	defer func() {
		for _, closer := range closers {
			_ = closer()
		}
	}()
	for _, arg := range os.Args[1:] {
		f, err := os.Open(arg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		closers = append(closers, f.Close)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println("awaiting signal")
	<-sigs
	fmt.Println("exiting")
}
