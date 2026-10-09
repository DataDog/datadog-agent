// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// gnmi-fakeserver runs a minimal gNMI server for local development and E2E tests.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/internal/fakeserver"
)

const (
	defaultListenAddr = "0.0.0.0:57400"
	defaultUsername   = "user"
	defaultPassword   = "test-password"
	defaultInterface  = "eth0"
)

func main() {
	listenAddr := flag.String("listen", envOrDefault("GNMI_FAKESERVER_LISTEN", defaultListenAddr), "TCP address to listen on")
	username := flag.String("username", envOrDefault("GNMI_FAKESERVER_USERNAME", defaultUsername), "expected gNMI username")
	password := flag.String("password", envOrDefault("GNMI_FAKESERVER_PASSWORD", defaultPassword), "expected gNMI password")
	interfaceName := flag.String("interface", envOrDefault("GNMI_FAKESERVER_INTERFACE", defaultInterface), "interface name for synthetic counters")
	flag.Parse()

	server, err := fakeserver.NewOn(*listenAddr)
	if err != nil {
		log.Fatalf("start fakeserver: %v", err)
	}
	defer server.Close()

	server.SetExpectedCredentials(*username, *password)
	log.Printf("gNMI fakeserver listening on %s", server.Addr())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go publishLoop(ctx, server, *interfaceName)

	<-ctx.Done()
}

func publishLoop(ctx context.Context, server *fakeserver.Server, interfaceName string) {
	var streamID int
	var inOctets uint64 = 42

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-server.SubscribeRequests():
			streamID = event.StreamID
			publishCounters(server, streamID, interfaceName, inOctets)
			inOctets += 10
		case <-time.After(1 * time.Second):
			if streamID == 0 {
				continue
			}
			publishCounters(server, streamID, interfaceName, inOctets)
			inOctets += 10
		}
	}
}

func publishCounters(server *fakeserver.Server, streamID int, interfaceName string, inOctets uint64) {
	updates := []*gnmipb.Update{
		fakeserver.InterfaceInOctetsUpdate(interfaceName, inOctets),
		fakeserver.InterfaceOutOctetsUpdate(interfaceName, inOctets*2),
		hostnameUpdate("gnmi-router-1"),
		interfaceNameUpdate(interfaceName),
	}
	for _, update := range updates {
		if err := server.SendUpdate(streamID, update); err != nil {
			log.Printf("publish update: %v", err)
			return
		}
	}
}

func hostnameUpdate(hostname string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "system"},
				{Name: "state"},
				{Name: "hostname"},
			},
		},
		Val: fakeserver.ScalarString(hostname),
	}
}

func interfaceNameUpdate(interfaceName string) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "name"},
			},
		},
		Val: fakeserver.ScalarString(interfaceName),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
