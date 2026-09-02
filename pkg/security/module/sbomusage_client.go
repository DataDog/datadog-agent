// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	sbomresolver "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// errNoUsageStream reports that the usage stream to the core agent is down, so
// what the resolver observed has nowhere to go until it comes back.
var errNoUsageStream = errors.New("no usage stream to the core agent")

const (
	// indexQueueSize bounds the indexes waiting for the resolver. Scans of
	// distinct workloads complete far apart, so a resolver that is up has no
	// queue at all.
	indexQueueSize = 16
	// reconnectDelay is how long to wait before dialling the core agent again.
	// The core agent may not be serving yet when system-probe starts.
	reconnectDelay = 5 * time.Second
)

// SBOMUsageClient is the core agent, seen from system-probe. It receives the
// file index of every scanned workload and sends back the usage the resolver
// observed against it.
//
// Both streams are opened from here because the index is the large artifact and
// it flows this way, so the side consuming it owns the connection and its
// lifetime.
type SBOMUsageClient struct {
	address string
	creds   credentials.TransportCredentials

	indexes chan *usage.Index

	mu           sync.RWMutex
	capabilities usage.Capabilities
	known        bool
	reports      pb.AgentSecure_SBOMReportUsageClient

	// sendMu serializes the sends. A gRPC stream takes one sender at a time, and
	// the periodic flush and a package-database write reach this from different
	// goroutines.
	sendMu sync.Mutex
}

var _ sbomresolver.IndexSource = (*SBOMUsageClient)(nil)

// NewSBOMUsageClient returns a client of the core agent's SBOM usage streams.
func NewSBOMUsageClient(ipcComp ipc.Component, cmdHost string, cmdPort int) *SBOMUsageClient {
	return &SBOMUsageClient{
		address: net.JoinHostPort(cmdHost, strconv.Itoa(cmdPort)),
		creds:   credentials.NewTLS(ipcComp.GetTLSClientConfig()),
		indexes: make(chan *usage.Index, indexQueueSize),
	}
}

// Start keeps the streams up until ctx is done.
func (c *SBOMUsageClient) Start(ctx context.Context) {
	go func() {
		for ctx.Err() == nil {
			if err := c.run(ctx); err != nil && ctx.Err() == nil {
				seclog.Debugf("SBOM index stream ended: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
			}
		}
	}()
}

// run holds one connection for as long as it lasts.
func (c *SBOMUsageClient) run(ctx context.Context) error {
	conn, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.creds))
	if err != nil {
		return fmt.Errorf("dial core agent: %w", err)
	}
	defer conn.Close()

	client := pb.NewAgentSecureClient(conn)

	reports, err := client.SBOMReportUsage(ctx)
	if err != nil {
		return fmt.Errorf("open usage stream: %w", err)
	}
	c.setReports(reports)
	defer c.setReports(nil)

	// Draining acknowledgements keeps the stream from stalling. New agents state
	// acceptance explicitly and identify the current index; for a legacy core
	// agent, preserve the former nonzero-generation interpretation.
	go func() {
		for {
			ack, err := reports.Recv()
			if err != nil {
				return
			}
			applied := ack.GetGeneration() != 0
			if ack.Applied != nil {
				applied = ack.GetApplied()
			}
			if !applied {
				seclog.Debugf("usage report for %s was not applied; current index is %q generation %d",
					ack.GetScanId(), ack.GetIndexId(), ack.GetGeneration())
			}
		}
	}()

	stream, err := client.SBOMStreamIndex(ctx, &pb.IndexRequest{})
	if err != nil {
		return fmt.Errorf("open index stream: %w", err)
	}

	for {
		update, err := stream.Recv()
		if err != nil {
			return err
		}

		switch body := update.GetBody().(type) {
		case *pb.IndexUpdate_Capabilities:
			c.setCapabilities(usage.CapabilitiesFromProto(body.Capabilities))
		case *pb.IndexUpdate_Index:
			select {
			case c.indexes <- usage.IndexFromProto(body.Index):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// Indexes implements sbomresolver.IndexSource.
func (c *SBOMUsageClient) Indexes() <-chan *usage.Index {
	return c.indexes
}

// Capabilities implements sbomresolver.IndexSource.
func (c *SBOMUsageClient) Capabilities() (usage.Capabilities, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capabilities, c.known
}

// Report implements sbomresolver.IndexSource.
func (c *SBOMUsageClient) Report(report *usage.Report) error {
	c.mu.RLock()
	stream := c.reports
	c.mu.RUnlock()

	if stream == nil {
		return errNoUsageStream
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return stream.Send(&pb.UsageMessage{
		Body: &pb.UsageMessage_Report{Report: usage.ReportToProto(report)},
	})
}

// Refresh implements sbomresolver.IndexSource.
func (c *SBOMUsageClient) Refresh(scan usage.ScanID, containerID containerutils.ContainerID) error {
	c.mu.RLock()
	stream := c.reports
	c.mu.RUnlock()

	if stream == nil {
		return errNoUsageStream
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return stream.Send(&pb.UsageMessage{
		Body: &pb.UsageMessage_Refresh{Refresh: &pb.RefreshRequest{
			ScanId:      string(scan),
			ContainerId: string(containerID),
		}},
	})
}

func (c *SBOMUsageClient) setCapabilities(capabilities usage.Capabilities) {
	c.mu.Lock()
	c.capabilities = capabilities
	c.known = true
	c.mu.Unlock()

	if !capabilities.Any() {
		seclog.Warnf("the core agent scans no workload, so the SBOM package fields and the runtime usage enrichment are unavailable")
		return
	}
	seclog.Infof("core agent SBOM sources: container_image=%t container=%t host=%t",
		capabilities.ContainerImage, capabilities.Container, capabilities.Host)
}

func (c *SBOMUsageClient) setReports(stream pb.AgentSecure_SBOMReportUsageClient) {
	c.mu.Lock()
	c.reports = stream
	c.mu.Unlock()
}
