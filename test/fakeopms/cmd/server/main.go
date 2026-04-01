// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	fakeopms "github.com/DataDog/datadog-agent/test/fakeopms/server"
)

func main() {
	portPtr := flag.Int("port", 8080, "fakeopms listening port, default 8080. Use -port=0 for a random available port")
	flag.Parse()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	ready := make(chan bool, 1)

	log.Println("⌛️ Starting fake OPMS")
	s := fakeopms.NewServer(
		fakeopms.WithPort(*portPtr),
		fakeopms.WithReadyChannel(ready),
	)
	s.Start()

	select {
	case ok := <-ready:
		if !ok {
			log.Println("Error starting fake OPMS server")
			return
		}
	}

	log.Printf("🏃 Fake OPMS running at %s", s.URL())
	<-sigs
	log.Println("Stopping fake OPMS")
	if err := s.Stop(); err != nil {
		log.Printf("Error stopping fake OPMS: %v", err)
	}
	log.Println("👋 Bye bye")
}
