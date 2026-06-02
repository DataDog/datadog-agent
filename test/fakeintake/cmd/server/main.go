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
	retentionPeriodPtr := flag.Duration("retention-period", 15*time.Minute, "data retention period (use format: 1m, 10s, 1h), default: 15 minutes")
	remoteConfig := flag.Bool("remoteconfig", true, "disable Remote Config endpoints (/api/v0.1/configurations etc.)")
	rcOrgUUID := flag.String("rc-org-uuid", "", "Remote Config: org UUID (default 42)")
	rcStatePath := flag.String("rc-state", "", "Remote Config: YAML file with initial config to preload")
	rcVersion := flag.Uint64("rc-version", 0, "Remote Config: initial version counter (default 1)")
	rcKeyPath := flag.String("rc-key-path", "", "Remote Config: ed25519 signing key path (default ~/.fakeintake/signing.key)")
	rcKeyData := flag.String("rc-key-data", "", "Remote Config: hex-encoded 32-byte ed25519 seed (takes precedence over --rc-key-path; use for ephemeral envs)")

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

	if retentionPeriodPtr != nil {
		fiOptions = append(fiOptions, fakeintake.WithRetention(*retentionPeriodPtr))
	}

	if *remoteConfig {
		fiOptions = append(fiOptions, fakeintake.WithRemoteConfig(*rcOrgUUID))
		if *rcKeyData != "" {
			fiOptions = append(fiOptions, fakeintake.WithRemoteConfigKeyData(*rcKeyData))
		} else if *rcKeyPath != "" {
			fiOptions = append(fiOptions, fakeintake.WithRemoteConfigKeyPath(*rcKeyPath))
		}
		if *rcVersion != 0 {
			fiOptions = append(fiOptions, fakeintake.WithRemoteConfigVersion(*rcVersion))
		}
		if *rcStatePath != "" {
			fiOptions = append(fiOptions, fakeintake.WithRemoteConfigInitialState(*rcStatePath))
		}
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
