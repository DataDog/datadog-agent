// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DescribeConsumerGroupHandler struct {
	newClient adminClientFactory
}

func NewDescribeConsumerGroupHandler() *DescribeConsumerGroupHandler {
	return &DescribeConsumerGroupHandler{newClient: newAdminClient}
}

type DescribeConsumerGroupInputs struct {
	Group string `json:"group"`
	// Topics, when set, limits offsets to these topics and includes their
	// partitions that have no committed offset.
	Topics []string `json:"topics,omitempty"`
}

type PartitionOffset struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	// CommittedOffset is -1 when the group has not committed an offset.
	CommittedOffset int64 `json:"committedOffset"`
	LogStartOffset  int64 `json:"logStartOffset"`
	LogEndOffset    int64 `json:"logEndOffset"`
	// Lag is -1 when it cannot be computed, in which case Error is set.
	Lag   int64  `json:"lag"`
	Error string `json:"error,omitempty"`
}

type DescribeConsumerGroupOutputs struct {
	ConsumerGroup
	Offsets  []PartitionOffset `json:"offsets"`
	TotalLag int64             `json:"totalLag"`
}

func (h *DescribeConsumerGroupHandler) Run(ctx context.Context, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	inputs, err := types.ExtractInputs[DescribeConsumerGroupInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}
	if inputs.Group == "" {
		return nil, errors.New("group is required")
	}

	client, err := h.newClient(credential)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	group, err := describeGroup(ctx, client, inputs.Group)
	if err != nil {
		return nil, err
	}

	var committed kadm.OffsetResponses
	if len(inputs.Topics) > 0 {
		committed, err = client.FetchOffsetsForTopics(ctx, inputs.Group, inputs.Topics...)
	} else {
		committed, err = client.FetchOffsets(ctx, inputs.Group)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to fetch committed offsets for consumer group %q: %w", inputs.Group, err)
	}

	offsets, totalLag, err := getPartitionOffsets(ctx, client, committed)
	if err != nil {
		return nil, err
	}

	return &DescribeConsumerGroupOutputs{
		ConsumerGroup: *group,
		Offsets:       offsets,
		TotalLag:      totalLag,
	}, nil
}

// getPartitionOffsets computes lag like kadm.CalculateGroupLagWithStartOffsets.
func getPartitionOffsets(ctx context.Context, client adminClient, committed kadm.OffsetResponses) ([]PartitionOffset, int64, error) {
	offsets := []PartitionOffset{}
	topics := committed.Partitions().Topics()
	if len(topics) == 0 {
		return offsets, 0, nil
	}
	startOffsets, err := listOffsets(ctx, client.ListStartOffsets, topics)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to list start offsets: %w", err)
	}
	endOffsets, err := listOffsets(ctx, client.ListEndOffsets, topics)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to list end offsets: %w", err)
	}

	var totalLag int64
	for _, commit := range committed.Sorted() {
		offset := PartitionOffset{
			Topic:           commit.Topic,
			Partition:       commit.Partition,
			CommittedOffset: commit.At,
			LogStartOffset:  -1,
			LogEndOffset:    -1,
			Lag:             -1,
		}
		start, hasStart := startOffsets.Lookup(commit.Topic, commit.Partition)
		if hasStart && start.Err == nil {
			offset.LogStartOffset = start.Offset
		}
		end, hasEnd := endOffsets.Lookup(commit.Topic, commit.Partition)
		switch {
		case commit.Err != nil:
			offset.Error = commit.Err.Error()
		case !hasEnd:
			offset.Error = "partition is missing from listed end offsets"
		case end.Err != nil:
			offset.Error = end.Err.Error()
		default:
			offset.LogEndOffset = end.Offset
			offset.Lag = end.Offset
			if offset.LogStartOffset >= 0 {
				offset.Lag = end.Offset - offset.LogStartOffset
			}
			if commit.At >= 0 {
				offset.Lag = end.Offset - commit.At
			}
			// A commit can be past the end offset, and segments can be
			// deleted between listing start and end offsets.
			offset.Lag = max(offset.Lag, 0)
			totalLag += offset.Lag
		}
		offsets = append(offsets, offset)
	}
	return offsets, totalLag, nil
}

// listOffsets tolerates brokers failing to answer when others did: their
// partitions are then missing from the result and reported per partition.
func listOffsets(ctx context.Context, list func(context.Context, ...string) (kadm.ListedOffsets, error), topics []string) (kadm.ListedOffsets, error) {
	listed, err := list(ctx, topics...)
	if shardErrs, ok := errors.AsType[*kadm.ShardErrors](err); ok && !shardErrs.AllFailed {
		return listed, nil
	}
	return listed, err
}
