// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package module

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// sbomStream hands over the messages GetSBOMStream sends.
type sbomStream struct {
	grpc.ServerStream
	sent chan *sbompb.SBOMMessage
}

func (s *sbomStream) Context() context.Context {
	return context.Background()
}

func (s *sbomStream) Send(msg *sbompb.SBOMMessage) error {
	s.sent <- msg
	return nil
}

// TestGetSBOMStreamKind checks that the report of the host, the one with an
// empty container ID, reaches the core agent under the host kind, and the
// report of a container under the container kind.
func TestGetSBOMStreamKind(t *testing.T) {
	tests := []struct {
		requestID string
		want      string
	}{
		{requestID: "", want: sbompkg.HostKind},
		{requestID: "0123456789ab", want: string(workloadmeta.KindContainer)},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			server := &SBOMAPIServer{
				sboms:    make(chan *sbompkg.ScanResult, 1),
				stopChan: make(chan struct{}),
			}
			server.sboms <- &sbompkg.ScanResult{
				Report:    sbom.NewPackagesReport(nil, containerutils.ContainerID(tt.requestID)),
				RequestID: tt.requestID,
			}

			stream := &sbomStream{sent: make(chan *sbompb.SBOMMessage, 1)}
			done := make(chan error)
			go func() {
				done <- server.GetSBOMStream(&sbompb.SBOMStreamParams{}, stream)
			}()

			var msg *sbompb.SBOMMessage
			select {
			case msg = <-stream.sent:
			case err := <-done:
				t.Fatalf("GetSBOMStream returned before sending: %v", err)
			}

			close(server.stopChan)
			if err := <-done; err != nil {
				t.Fatalf("GetSBOMStream: %v", err)
			}

			if msg.Kind != tt.want {
				t.Errorf("kind = %q, want %q", msg.Kind, tt.want)
			}
			if msg.ID != tt.requestID {
				t.Errorf("ID = %q, want %q", msg.ID, tt.requestID)
			}
		})
	}
}
