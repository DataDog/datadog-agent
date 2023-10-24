// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

const (
	srvAddr = "127.0.0.1:5050"
)

func main() {
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT)

	srv, err := grpc.NewServer(srvAddr, true)
	if err != nil {
		os.Exit(1)
	}

	srv.Run()

	<-done
}
