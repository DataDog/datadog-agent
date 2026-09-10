// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package server implements the gRPC streams that hand the SBOM file index to a
// runtime observer and take back the usage it observed.
package server

import (
	"errors"
	"io"

	usagedef "github.com/DataDog/datadog-agent/comp/sbom/usage/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

// subscriberQueueSize bounds the indexes waiting for one consumer. Scans of
// distinct workloads complete far apart, so a consumer that is up has no queue.
const subscriberQueueSize = 16

// Server serves the SBOM usage streams of one usage component.
type Server struct {
	usage usagedef.Component
}

// NewServer returns a server backed by the given usage component.
func NewServer(usage usagedef.Component) *Server {
	return &Server{usage: usage}
}

// StreamIndex sends the capability set, then every index the agent has already
// published, then the ones it publishes next.
//
// The capabilities come first and unconditionally: a consumer has to learn that
// no scan source is running, which is a different answer from an index that has
// not arrived yet, and it cannot read the agent's configuration to find out.
func (s *Server) StreamIndex(_ *pb.IndexRequest, out pb.AgentSecure_SBOMStreamIndexServer) error {
	if s.usage == nil {
		return errors.New("sbom usage enrichment is not enabled")
	}

	capabilities := &pb.IndexUpdate_Capabilities{
		Capabilities: usage.CapabilitiesToProto(s.usage.Capabilities()),
	}
	if err := out.Send(&pb.IndexUpdate{Body: capabilities}); err != nil {
		return err
	}

	snapshot, indexes, release := s.usage.Subscribe(subscriberQueueSize)
	defer release()

	for _, index := range snapshot {
		if err := sendIndex(out, index); err != nil {
			return err
		}
	}

	for {
		select {
		case <-out.Context().Done():
			return nil
		case index := <-indexes:
			if err := sendIndex(out, index); err != nil {
				return err
			}
		}
	}
}

// sendIndex writes one index.
func sendIndex(out pb.AgentSecure_SBOMStreamIndexServer, index *usage.Index) error {
	return out.Send(&pb.IndexUpdate{
		Body: &pb.IndexUpdate_Index{Index: usage.IndexToProto(index)},
	})
}

// ReportUsage records the usage a consumer observed and acknowledges whether
// the exact BOM/index instance accepted it.
//
// A refresh acknowledgement has applied=true and no index identity; the new
// index arrives on the other stream once the rescan completes.
func (s *Server) ReportUsage(stream pb.AgentSecure_SBOMReportUsageServer) error {
	if s.usage == nil {
		return errors.New("sbom usage enrichment is not enabled")
	}

	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		switch body := msg.GetBody().(type) {
		case *pb.UsageMessage_Report:
			report := usage.ReportFromProto(body.Report)
			ack := s.usage.Report(report)
			applied := ack.Applied
			if err := stream.Send(&pb.UsageAck{
				ScanId:     string(ack.Scan),
				Generation: ack.Generation,
				IndexId:    ack.IndexID,
				Applied:    &applied,
			}); err != nil {
				return err
			}
		case *pb.UsageMessage_Refresh:
			s.usage.Refresh(usage.ScanID(body.Refresh.GetScanId()), body.Refresh.GetContainerId())
			applied := true
			if err := stream.Send(&pb.UsageAck{ScanId: body.Refresh.GetScanId(), Applied: &applied}); err != nil {
				return err
			}
		}
	}
}
