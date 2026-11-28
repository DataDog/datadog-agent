// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"

	testhelpers "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/test"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	testGroup = "orders-validator"
	testTopic = "orders-fulfillment"
)

type commitCall struct {
	group   string
	offsets kadm.Offsets
}

// fakeAdminClient serves canned cluster state and records offset commits.
type fakeAdminClient struct {
	metadata    kadm.Metadata
	metadataErr error

	kip848Unsupported bool
	apiVersionsErr    error

	groups            kadm.DescribedGroups
	groupsErr         error
	consumerGroups    kadm.DescribedConsumerGroups
	consumerGroupsErr error

	committed    kadm.OffsetResponses
	fetchErr     error
	startOffsets kadm.ListedOffsets
	endOffsets   kadm.ListedOffsets

	commitResponses kadm.OffsetResponses
	commitErr       error
	commits         []commitCall

	closed bool
}

func (f *fakeAdminClient) factory() adminClientFactory {
	return func(*privateconnection.PrivateCredentials) (adminClient, error) {
		return f, nil
	}
}

func (f *fakeAdminClient) BrokerMetadata(context.Context) (kadm.Metadata, error) {
	return f.metadata, f.metadataErr
}

func (f *fakeAdminClient) SupportsConsumerGroupDescribe(context.Context) (bool, error) {
	return !f.kip848Unsupported, f.apiVersionsErr
}

func (f *fakeAdminClient) DescribeGroups(_ context.Context, groups ...string) (kadm.DescribedGroups, error) {
	if f.groupsErr != nil {
		return nil, f.groupsErr
	}
	described := make(kadm.DescribedGroups)
	for _, group := range groups {
		if g, ok := f.groups[group]; ok {
			described[group] = g
		} else {
			// Brokers that predate KIP-1043 report unknown groups as Dead without an error.
			described[group] = kadm.DescribedGroup{Group: group, State: groupStateDead}
		}
	}
	return described, nil
}

func (f *fakeAdminClient) DescribeConsumerGroups(_ context.Context, groups ...string) (kadm.DescribedConsumerGroups, error) {
	if f.consumerGroupsErr != nil {
		return nil, f.consumerGroupsErr
	}
	described := make(kadm.DescribedConsumerGroups)
	for _, group := range groups {
		if g, ok := f.consumerGroups[group]; ok {
			described[group] = g
		} else {
			described[group] = kadm.DescribedConsumerGroup{Group: group, Err: kerr.GroupIDNotFound}
		}
	}
	return described, nil
}

func (f *fakeAdminClient) FetchOffsets(context.Context, string) (kadm.OffsetResponses, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	responses := make(kadm.OffsetResponses)
	f.committed.Each(responses.Add)
	return responses, nil
}

func (f *fakeAdminClient) FetchOffsetsForTopics(_ context.Context, _ string, topics ...string) (kadm.OffsetResponses, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	responses := make(kadm.OffsetResponses)
	for _, topic := range topics {
		for partition := range f.endOffsets[topic] {
			responses.Add(kadm.OffsetResponse{Offset: kadm.Offset{Topic: topic, Partition: partition, At: -1, LeaderEpoch: -1}})
		}
		for _, committed := range f.committed[topic] {
			responses.Add(committed)
		}
	}
	return responses, nil
}

func (f *fakeAdminClient) ListStartOffsets(_ context.Context, topics ...string) (kadm.ListedOffsets, error) {
	return filterListedOffsets(f.startOffsets, topics), nil
}

func (f *fakeAdminClient) ListEndOffsets(_ context.Context, topics ...string) (kadm.ListedOffsets, error) {
	return filterListedOffsets(f.endOffsets, topics), nil
}

func (f *fakeAdminClient) CommitOffsets(_ context.Context, group string, offsets kadm.Offsets) (kadm.OffsetResponses, error) {
	f.commits = append(f.commits, commitCall{group: group, offsets: offsets})
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	if f.commitResponses != nil {
		return f.commitResponses, nil
	}
	responses := make(kadm.OffsetResponses)
	offsets.Each(func(o kadm.Offset) {
		responses.Add(kadm.OffsetResponse{Offset: o})
	})
	return responses, nil
}

func (f *fakeAdminClient) Close() {
	f.closed = true
}

// filterListedOffsets mimics kadm, which lists unknown topics with a -1 partition.
func filterListedOffsets(all kadm.ListedOffsets, topics []string) kadm.ListedOffsets {
	listed := make(kadm.ListedOffsets)
	for _, topic := range topics {
		if partitions, ok := all[topic]; ok {
			listed[topic] = partitions
		} else {
			listed[topic] = map[int32]kadm.ListedOffset{-1: {Topic: topic, Partition: -1, Err: kerr.UnknownTopicOrPartition}}
		}
	}
	return listed
}

func newListedOffsets(topic string, offsets ...int64) kadm.ListedOffsets {
	partitions := make(map[int32]kadm.ListedOffset, len(offsets))
	for partition, offset := range offsets {
		partitions[int32(partition)] = kadm.ListedOffset{Topic: topic, Partition: int32(partition), Offset: offset, LeaderEpoch: -1}
	}
	return kadm.ListedOffsets{topic: partitions}
}

func newCommittedOffsets(topic string, offsets map[int32]int64) kadm.OffsetResponses {
	responses := make(kadm.OffsetResponses)
	for partition, offset := range offsets {
		responses.Add(kadm.OffsetResponse{Offset: kadm.Offset{Topic: topic, Partition: partition, At: offset, LeaderEpoch: -1}})
	}
	return responses
}

func newClassicGroup(state string, members ...kadm.DescribedGroupMember) kadm.DescribedGroups {
	return kadm.DescribedGroups{testGroup: {
		Group:        testGroup,
		Coordinator:  kadm.BrokerDetail{NodeID: 1},
		State:        state,
		ProtocolType: "consumer",
		Protocol:     "range",
		Members:      members,
	}}
}

func newTestTask(action string, inputs map[string]any) *types.Task {
	return testhelpers.NewTestTask("id", "type", &types.Attributes{
		Name:     action,
		BundleID: "com.datadoghq.kafka",
		Inputs:   inputs,
		OrgId:    123,
	})
}
