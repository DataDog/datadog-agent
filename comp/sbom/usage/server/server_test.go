// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"io"
	"testing"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"google.golang.org/grpc"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

type fakeUsage struct {
	ack       usage.ReportAck
	reported  *usage.Report
	refreshed usage.ScanID
}

func (*fakeUsage) Stamp(_ usage.ScanID, bom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom { return bom }
func (*fakeUsage) Revision(usage.ScanID) uint64                                      { return 0 }
func (*fakeUsage) Subscribe(int) ([]*usage.Index, <-chan *usage.Index, func()) {
	return nil, make(chan *usage.Index), func() {}
}
func (f *fakeUsage) Report(report *usage.Report) usage.ReportAck {
	f.reported = report
	return f.ack
}
func (*fakeUsage) Capabilities() usage.Capabilities { return usage.Capabilities{} }
func (f *fakeUsage) Refresh(scan usage.ScanID, _ string) {
	f.refreshed = scan
}

type reportStream struct {
	grpc.ServerStream
	messages []*pb.UsageMessage
	acks     []*pb.UsageAck
}

func (s *reportStream) Recv() (*pb.UsageMessage, error) {
	if len(s.messages) == 0 {
		return nil, io.EOF
	}
	message := s.messages[0]
	s.messages = s.messages[1:]
	return message, nil
}

func (s *reportStream) Send(ack *pb.UsageAck) error {
	s.acks = append(s.acks, ack)
	return nil
}

func TestReportUsageAcknowledgesApplicationExplicitly(t *testing.T) {
	backend := &fakeUsage{ack: usage.ReportAck{
		Scan: "image:x", Generation: 3, IndexID: "urn:uuid:current", Applied: false,
	}}
	stream := &reportStream{messages: []*pb.UsageMessage{
		{Body: &pb.UsageMessage_Report{Report: &pb.UsageReport{
			ScanId: "image:x", Generation: 2, IndexId: "urn:uuid:stale",
		}}},
		{Body: &pb.UsageMessage_Refresh{Refresh: &pb.RefreshRequest{ScanId: "image:x"}}},
	}}

	if err := NewServer(backend).ReportUsage(stream); err != nil {
		t.Fatal(err)
	}
	if backend.reported == nil || backend.reported.IndexID != "urn:uuid:stale" {
		t.Fatalf("reported identity = %#v", backend.reported)
	}
	if backend.refreshed != "image:x" {
		t.Errorf("refreshed = %q, want image:x", backend.refreshed)
	}
	if len(stream.acks) != 2 {
		t.Fatalf("acks = %d, want 2", len(stream.acks))
	}
	reportAck := stream.acks[0]
	if reportAck.Applied == nil || reportAck.GetApplied() || reportAck.GetIndexId() != "urn:uuid:current" || reportAck.GetGeneration() != 3 {
		t.Errorf("report ack = %#v", reportAck)
	}
	refreshAck := stream.acks[1]
	if refreshAck.Applied == nil || !refreshAck.GetApplied() || refreshAck.GetIndexId() != "" || refreshAck.GetGeneration() != 0 {
		t.Errorf("refresh ack = %#v", refreshAck)
	}
}
