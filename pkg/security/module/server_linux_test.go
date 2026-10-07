// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// TestPendingMsgIsResolvedSSHSession makes sure that an event of an SSH session waits for its
// authentication log line to be tailed, but only for a bounded number of retries, and only once per
// session: the retry queue is ordered, so waiting longer would delay all the following events.
func TestPendingMsgIsResolvedSSHSession(t *testing.T) {
	resolver, err := usersessions.NewResolver(64, true)
	require.NoError(t, err)

	newPatcher := func() sshSessionPatcher {
		return probe.NewSSHUserSessionPatcher(
			&serializers.SSHSessionContextSerializer{
				SSHClientIP:   "127.0.0.1",
				SSHClientPort: 38835,
			},
			resolver,
			4242,
		)
	}
	newMsg := func(retry int) *pendingMsg {
		return &pendingMsg{
			ruleID:            "test-rule",
			retry:             retry,
			sshSessionPatcher: newPatcher(),
		}
	}
	maxRetry := newPatcher().MaxRetry()

	// the auth log line has not been tailed yet, the event must be held back
	assert.False(t, newMsg(0).isResolved(), "event must wait for the ssh session to be resolved")
	assert.False(t, newMsg(maxRetry-1).isResolved(), "event must wait until its retry budget is exhausted")

	// budget exhausted: the event is sent as is, and the session is flagged so that the next events
	// of the same session are not delayed anymore
	key := usersessions.SSHSessionKey{SSHDPid: "4242", IP: "127.0.0.1", Port: "38835"}
	assert.False(t, resolver.IsSSHSessionUnresolved(key))
	assert.True(t, newMsg(maxRetry).isResolved(), "event must be sent once the retry budget is exhausted")
	assert.True(t, resolver.IsSSHSessionUnresolved(key), "the ssh session must be flagged as unresolved")

	// following events of that session must not be delayed
	assert.True(t, newMsg(0).isResolved(), "events of an unresolved session must not be delayed")
}

// sbomStream hands over the messages GetSBOMStream sends, or fails to send
// them when broken.
type sbomStream struct {
	grpc.ServerStream
	sent   chan *sbompb.SBOMMessage
	broken bool
}

func (s *sbomStream) Context() context.Context {
	return context.Background()
}

func (s *sbomStream) Send(msg *sbompb.SBOMMessage) error {
	if s.broken {
		return errors.New("broken stream")
	}
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
				sboms:    newSBOMQueue(1),
				stopChan: make(chan struct{}),
			}
			server.sboms.push(&sbompkg.ScanResult{
				Report:    sbom.NewPackagesReport(nil, containerutils.ContainerID(tt.requestID)),
				RequestID: tt.requestID,
			})

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

// TestGetSBOMStreamRequeuesFailedSend checks that a report the stream fails to
// send waits for the next stream, as the core agent reconnects.
func TestGetSBOMStreamRequeuesFailedSend(t *testing.T) {
	server := &SBOMAPIServer{
		sboms:    newSBOMQueue(1),
		stopChan: make(chan struct{}),
	}
	server.sboms.push(&sbompkg.ScanResult{
		Report:    sbom.NewPackagesReport(nil, "0123456789ab"),
		RequestID: "0123456789ab",
	})

	broken := &sbomStream{broken: true}
	if err := server.GetSBOMStream(&sbompb.SBOMStreamParams{}, broken); err == nil {
		t.Fatalf("GetSBOMStream returned no error on a broken stream")
	}

	stream := &sbomStream{sent: make(chan *sbompb.SBOMMessage, 1)}
	done := make(chan error)
	go func() {
		done <- server.GetSBOMStream(&sbompb.SBOMStreamParams{}, stream)
	}()

	select {
	case msg := <-stream.sent:
		if msg.ID != "0123456789ab" {
			t.Errorf("ID = %q, want 0123456789ab", msg.ID)
		}
	case err := <-done:
		t.Fatalf("GetSBOMStream returned before sending: %v", err)
	}

	close(server.stopChan)
	if err := <-done; err != nil {
		t.Fatalf("GetSBOMStream: %v", err)
	}
}
