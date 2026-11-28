// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
)

// newStalledGroupClient models a group stuck on a poison record at offset 20
// of partition 0, after its consumers were scaled to zero.
func newStalledGroupClient() *fakeAdminClient {
	return &fakeAdminClient{
		groups:       newClassicGroup(groupStateEmpty),
		committed:    newCommittedOffsets(testTopic, map[int32]int64{0: 20, 1: 35}),
		startOffsets: newListedOffsets(testTopic, 0, 0),
		endOffsets:   newListedOffsets(testTopic, 42, 35),
	}
}

func resetInputs(partition, offset int) map[string]any {
	return map[string]any{"group": testGroup, "topic": testTopic, "partition": partition, "offset": offset}
}

func TestResetConsumerGroupOffsetSkipsPoisonRecord(t *testing.T) {
	client := newStalledGroupClient()
	handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", resetInputs(0, 21)), nil)
	require.NoError(t, err)

	assert.Equal(t, &ResetConsumerGroupOffsetOutputs{
		Group:          testGroup,
		Topic:          testTopic,
		Partition:      0,
		PreviousOffset: 20,
		Offset:         21,
		LogStartOffset: 0,
		LogEndOffset:   42,
	}, output)
	require.Len(t, client.commits, 1)
	assert.Equal(t, testGroup, client.commits[0].group)
	assert.Equal(t, kadm.Offsets{testTopic: {0: {Topic: testTopic, Partition: 0, At: 21, LeaderEpoch: -1}}}, client.commits[0].offsets)
	assert.True(t, client.closed)
}

func TestResetConsumerGroupOffsetWithoutPreviousCommit(t *testing.T) {
	client := newStalledGroupClient()
	client.committed = nil
	handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", resetInputs(1, 35)), nil)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), output.(*ResetConsumerGroupOffsetOutputs).PreviousOffset)
	assert.Len(t, client.commits, 1)
}

func TestResetConsumerGroupOffsetToLogStart(t *testing.T) {
	client := newStalledGroupClient()
	client.startOffsets = newListedOffsets(testTopic, 10, 0)
	handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", resetInputs(0, 10)), nil)
	require.NoError(t, err)
	assert.Equal(t, int64(10), output.(*ResetConsumerGroupOffsetOutputs).Offset)
	assert.Len(t, client.commits, 1)
}

func TestResetConsumerGroupOffsetConsumerProtocolGroup(t *testing.T) {
	client := newStalledGroupClient()
	client.groups = kadm.DescribedGroups{testGroup: {Group: testGroup, Err: kerr.GroupIDNotFound}}
	client.consumerGroups = kadm.DescribedConsumerGroups{testGroup: {Group: testGroup, State: groupStateEmpty}}
	handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

	_, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", resetInputs(0, 21)), nil)
	require.NoError(t, err)
	assert.Len(t, client.commits, 1)
}

func TestResetConsumerGroupOffsetRefusals(t *testing.T) {
	tests := []struct {
		name    string
		inputs  map[string]any
		setup   func(client *fakeAdminClient)
		wantErr string
	}{
		{name: "missing group", inputs: map[string]any{"topic": testTopic, "partition": 0, "offset": 21}, wantErr: "group is required"},
		{name: "missing topic", inputs: map[string]any{"group": testGroup, "partition": 0, "offset": 21}, wantErr: "topic is required"},
		{name: "missing partition", inputs: map[string]any{"group": testGroup, "topic": testTopic, "offset": 21}, wantErr: "partition is required"},
		{name: "negative partition", inputs: resetInputs(-1, 21), wantErr: "partition -1 must not be negative"},
		{name: "missing offset", inputs: map[string]any{"group": testGroup, "topic": testTopic, "partition": 0}, wantErr: "offset is required"},
		{name: "negative offset", inputs: resetInputs(0, -2), wantErr: "offset -2 must not be negative"},
		{name: "non numeric offset", inputs: map[string]any{"group": testGroup, "topic": testTopic, "partition": 0, "offset": "latest"}, wantErr: "failed to extract inputs"},
		{
			name:   "active classic group",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.groups = newClassicGroup("Stable", kadm.DescribedGroupMember{MemberID: "member-1"}, kadm.DescribedGroupMember{MemberID: "member-2"})
			},
			wantErr: `consumer group "orders-validator" must be inactive to reset its offsets, but it is Stable with 2 member(s)`,
		},
		{
			name:   "rebalancing classic group",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.groups = newClassicGroup("PreparingRebalance")
			},
			wantErr: "but it is PreparingRebalance with 0 member(s)",
		},
		{
			name:   "active consumer protocol group",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.groups = nil
				client.consumerGroups = kadm.DescribedConsumerGroups{testGroup: {
					Group: testGroup, State: "Reconciling", Members: []kadm.ConsumerGroupMember{{MemberID: "member-1"}},
				}}
			},
			wantErr: "but it is Reconciling with 1 member(s)",
		},
		{
			name:   "unknown group",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.groups = nil
			},
			wantErr: `consumer group "orders-validator" does not exist`,
		},
		{name: "offset past the log end", inputs: resetInputs(0, 43), wantErr: "offset 43 is out of range for topic \"orders-fulfillment\" partition 0: valid offsets are 0 to 42"},
		{
			name:   "offset before the log start",
			inputs: resetInputs(0, 5),
			setup: func(client *fakeAdminClient) {
				client.startOffsets = newListedOffsets(testTopic, 10, 0)
			},
			wantErr: "valid offsets are 10 to 42",
		},
		{name: "unknown partition", inputs: resetInputs(7, 21), wantErr: `topic "orders-fulfillment" partition 7 does not exist`},
		{name: "unknown topic", inputs: map[string]any{"group": testGroup, "topic": "orders", "partition": 0, "offset": 21}, wantErr: `topic "orders" partition 0 does not exist`},
		{
			name:   "partition offsets unavailable",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.endOffsets = kadm.ListedOffsets{testTopic: {0: {Topic: testTopic, Partition: 0, Err: kerr.NotLeaderForPartition}}}
			},
			wantErr: "unable to list offsets for topic \"orders-fulfillment\" partition 0",
		},
		{
			name:   "committed offsets unavailable",
			inputs: resetInputs(0, 21),
			setup: func(client *fakeAdminClient) {
				client.fetchErr = errors.New("coordinator unavailable")
			},
			wantErr: "unable to fetch committed offsets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newStalledGroupClient()
			if tt.setup != nil {
				tt.setup(client)
			}
			handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

			_, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", tt.inputs), nil)
			require.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, client.commits)
		})
	}
}

func TestResetConsumerGroupOffsetCommitFailures(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(client *fakeAdminClient)
		wantErr string
	}{
		{
			name: "consumers joined after the state check",
			setup: func(client *fakeAdminClient) {
				client.commitResponses = kadm.OffsetResponses{testTopic: {0: {Offset: kadm.Offset{Topic: testTopic, Partition: 0, At: 21}, Err: kerr.UnknownMemberID}}}
			},
			wantErr: `consumer group "orders-validator" has active members, scale its consumers to zero and retry`,
		},
		{
			name: "commit not authorized",
			setup: func(client *fakeAdminClient) {
				client.commitResponses = kadm.OffsetResponses{testTopic: {0: {Offset: kadm.Offset{Topic: testTopic, Partition: 0, At: 21}, Err: kerr.TopicAuthorizationFailed}}}
			},
			wantErr: "TOPIC_AUTHORIZATION_FAILED",
		},
		{
			name: "request failure",
			setup: func(client *fakeAdminClient) {
				client.commitErr = errors.New("connection reset")
			},
			wantErr: `unable to commit offset for consumer group "orders-validator": connection reset`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newStalledGroupClient()
			tt.setup(client)
			handler := &ResetConsumerGroupOffsetHandler{newClient: client.factory()}

			_, err := handler.Run(context.Background(), newTestTask("resetConsumerGroupOffset", resetInputs(0, 21)), nil)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
