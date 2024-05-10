// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main is a utility to mmap libraries to test shared library handling.
package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/exp/mmap"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: libmmap <filename>....")
		os.Exit(1)
	}

	for {
		// To allow time to attach
		time.Sleep(1000 * time.Millisecond)

		f, err := mmap.Open((os.Args[1]))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		f.Close()
	}
}
