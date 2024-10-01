// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APL) Fix revive linter
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

func main() {
	portPtr := flag.Int("port", 80, "fakeintake listening port, default to 80. Using -port=0 will use a random available port")
	dddevForward := flag.Bool("dddev-forward", false, "Forward POST payloads to dddev, using the env variable DD_API_KEY as API key")
	flag.Parse()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	ready := make(chan bool, 1)
	fiOptions := []fakeintake.Option{
		fakeintake.WithPort(*portPtr),
		fakeintake.WithReadyChannel(ready),
	}
	if *dddevForward {
		fiOptions = append(fiOptions, fakeintake.WithDDDevForward())
	}
	log.Println("⌛️ Starting fake intake")
	fi := fakeintake.NewServer(fiOptions...)
	fi.Start()
	timeout := time.NewTimer(5 * time.Second)

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

	log.Printf("🏃 Fake intake running at %s", fi.URL())
	<-sigs
	log.Println("Stopping fake intake")
	err := fi.Stop()
	if err != nil {
		log.Println("Error stopping fake intake, ", err)
	}

	log.Println("Fake intake is stopped")
	log.Println("👋 Bye bye")

}
