// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c)
	s := <-c
	if s != syscall.SIGTERM {
		fmt.Errorf("Wrong signal: %v, expected: %v", s, syscall.SIGTERM)
		os.Exit(2)
	}
	fmt.Println("I've been terminated!")
	os.Exit(1)
}
