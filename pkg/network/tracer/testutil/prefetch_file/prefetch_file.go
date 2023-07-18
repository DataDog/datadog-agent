// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: prefetch_file <filename> <wait_time>")
		os.Exit(1)
	}
	waitTime, err := time.ParseDuration(os.Args[2])
	if err != nil {
		fmt.Printf("%s is not a valid format of duration\n", os.Args[2])
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	time.Sleep(waitTime)
}
