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

func TestDescribeConsumerGroup(t *testing.T) {
	client := &fakeAdminClient{
		groups: newClassicGroup("Stable", kadm.DescribedGroupMember{MemberID: "member-1", ClientID: "validator", ClientHost: "/10.0.0.1"}),
		committed: kadm.OffsetResponses{testTopic: {
			0: {Offset: kadm.Offset{Topic: testTopic, Partition: 0, At: 20}},
			1: {Offset: kadm.Offset{Topic: testTopic, Partition: 1, At: 50}},
			2: {Offset: kadm.Offset{Topic: testTopic, Partition: 2, At: -1}, Err: kerr.UnstableOffsetCommit},
		}},
		startOffsets: newListedOffsets(testTopic, 0, 0, 0),
		endOffsets:   newListedOffsets(testTopic, 42, 35, 7),
	}
	handler := &DescribeConsumerGroupHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{"group": testGroup}), nil)
	require.NoError(t, err)

	assert.Equal(t, &DescribeConsumerGroupOutputs{
		ConsumerGroup: ConsumerGroup{
			Group:         testGroup,
			State:         "Stable",
			GroupProtocol: classicGroupProtocol,
			ProtocolType:  "consumer",
			CoordinatorID: int32Ptr(1),
			Members:       []GroupMember{{MemberID: "member-1", ClientID: "validator", ClientHost: "/10.0.0.1", Assignments: []TopicPartitions{}}},
		},
		Offsets: []PartitionOffset{
			{Topic: testTopic, Partition: 0, CommittedOffset: 20, LogStartOffset: 0, LogEndOffset: 42, Lag: 22},
			// A commit past the end offset has no lag.
			{Topic: testTopic, Partition: 1, CommittedOffset: 50, LogStartOffset: 0, LogEndOffset: 35, Lag: 0},
			{Topic: testTopic, Partition: 2, CommittedOffset: -1, LogStartOffset: 0, LogEndOffset: -1, Lag: -1, Error: kerr.UnstableOffsetCommit.Error()},
		},
		TotalLag: 22,
	}, output)
	assert.True(t, client.closed)
}

func TestDescribeConsumerGroupFilteredByTopic(t *testing.T) {
	client := &fakeAdminClient{
		groups:       newClassicGroup(groupStateEmpty),
		committed:    newCommittedOffsets(testTopic, map[int32]int64{0: 20}),
		startOffsets: newListedOffsets(testTopic, 0, 10),
		endOffsets:   newListedOffsets(testTopic, 42, 15),
	}
	handler := &DescribeConsumerGroupHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{"group": testGroup, "topics": []string{testTopic}}), nil)
	require.NoError(t, err)

	outputs := output.(*DescribeConsumerGroupOutputs)
	assert.Equal(t, groupStateEmpty, outputs.State)
	assert.Empty(t, outputs.Members)
	assert.Equal(t, []PartitionOffset{
		{Topic: testTopic, Partition: 0, CommittedOffset: 20, LogStartOffset: 0, LogEndOffset: 42, Lag: 22},
		// Without a commit, lag is counted from the log start offset.
		{Topic: testTopic, Partition: 1, CommittedOffset: -1, LogStartOffset: 10, LogEndOffset: 15, Lag: 5},
	}, outputs.Offsets)
	assert.Equal(t, int64(27), outputs.TotalLag)
}

func TestDescribeConsumerGroupWithoutCommits(t *testing.T) {
	client := &fakeAdminClient{}
	handler := &DescribeConsumerGroupHandler{newClient: client.factory()}

	output, err := handler.Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{"group": testGroup}), nil)
	require.NoError(t, err)

	outputs := output.(*DescribeConsumerGroupOutputs)
	assert.Equal(t, groupStateDead, outputs.State)
	assert.Equal(t, []PartitionOffset{}, outputs.Offsets)
	assert.Zero(t, outputs.TotalLag)
}

func TestDescribeConsumerGroupErrors(t *testing.T) {
	_, err := (&DescribeConsumerGroupHandler{newClient: (&fakeAdminClient{}).factory()}).Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{}), nil)
	require.ErrorContains(t, err, "group is required")

	_, err = (&DescribeConsumerGroupHandler{newClient: newAdminClient}).Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{"group": testGroup}), nil)
	require.ErrorContains(t, err, "missing Kafka connection")

	client := &fakeAdminClient{fetchErr: errors.New("coordinator unavailable")}
	_, err = (&DescribeConsumerGroupHandler{newClient: client.factory()}).Run(context.Background(), newTestTask("describeConsumerGroup", map[string]any{"group": testGroup}), nil)
	require.ErrorContains(t, err, `unable to fetch committed offsets for consumer group "orders-validator": coordinator unavailable`)
}

func TestListOffsetsToleratesPartialShardErrors(t *testing.T) {
	listed := newListedOffsets(testTopic, 42)
	partial := func(context.Context, ...string) (kadm.ListedOffsets, error) {
		return listed, &kadm.ShardErrors{Name: "ListOffsets", Errs: []kadm.ShardError{{Err: errors.New("broker 2 unavailable")}}}
	}
	got, err := listOffsets(context.Background(), partial, []string{testTopic})
	require.NoError(t, err)
	assert.Equal(t, listed, got)

	allFailed := func(context.Context, ...string) (kadm.ListedOffsets, error) {
		return nil, &kadm.ShardErrors{Name: "ListOffsets", AllFailed: true, Errs: []kadm.ShardError{{Err: errors.New("cluster unavailable")}}}
	}
	_, err = listOffsets(context.Background(), allFailed, []string{testTopic})
	require.Error(t, err)
}
