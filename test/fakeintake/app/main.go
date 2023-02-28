// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	log.Println("⌛️ Starting fake intake")
	ready := make(chan bool, 1)
	fi := fakeintake.NewServer(fakeintake.WithPort(8080), fakeintake.WithReadyChannel(ready))
	fi.Start()
	timeout := time.NewTimer(250 * time.Millisecond)

	select {
	case isReady := <-ready:
		if !isReady {
			log.Println("Error starting fake intake")
			return
		}
	case <-timeout.C:
		log.Println("Error starting server, not ready after 250 ms")
		return
	}
	timeout.Stop()

	log.Println("🏃 Fake intake running")

	<-sigs
	log.Println("Stopping fake intake")
	err := fi.Stop()
	if err != nil {
		log.Println("Error stopping fake intake, ", err)
	}

	log.Println("Fake intake is stopped")
	log.Println("👋 Bye bye")
}
