// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ResetConsumerGroupOffsetHandler struct {
	newClient adminClientFactory
}

func NewResetConsumerGroupOffsetHandler() *ResetConsumerGroupOffsetHandler {
	return &ResetConsumerGroupOffsetHandler{newClient: newAdminClient}
}

type ResetConsumerGroupOffsetInputs struct {
	Group     string `json:"group"`
	Topic     string `json:"topic"`
	Partition *int32 `json:"partition"`
	// Offset is the offset of the next record the group consumes.
	Offset *int64 `json:"offset"`
}

type ResetConsumerGroupOffsetOutputs struct {
	Group     string `json:"group"`
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	// PreviousOffset is -1 when the group had no committed offset.
	PreviousOffset int64 `json:"previousOffset"`
	Offset         int64 `json:"offset"`
	LogStartOffset int64 `json:"logStartOffset"`
	LogEndOffset   int64 `json:"logEndOffset"`
}

func (h *ResetConsumerGroupOffsetHandler) Run(ctx context.Context, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	inputs, err := types.ExtractInputs[ResetConsumerGroupOffsetInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to extract inputs: %w", err)
	}
	if err := inputs.validate(); err != nil {
		return nil, err
	}
	group, topic, partition, offset := inputs.Group, inputs.Topic, *inputs.Partition, *inputs.Offset

	client, err := h.newClient(credential)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	described, err := describeGroup(ctx, client, group)
	if err != nil {
		return nil, err
	}
	if err := checkGroupIsInactive(described); err != nil {
		return nil, err
	}

	logStartOffset, logEndOffset, err := getPartitionBounds(ctx, client, topic, partition)
	if err != nil {
		return nil, err
	}
	if offset < logStartOffset || offset > logEndOffset {
		return nil, fmt.Errorf("offset %d is out of range for topic %q partition %d: valid offsets are %d to %d", offset, topic, partition, logStartOffset, logEndOffset)
	}

	committed, err := client.FetchOffsets(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch committed offsets for consumer group %q: %w", group, err)
	}
	previousOffset := int64(-1)
	if previous, ok := committed.Lookup(topic, partition); ok && previous.Err == nil {
		previousOffset = previous.At
	}

	log.FromContext(ctx).Info("Resetting Kafka consumer group offset",
		log.String("group", group),
		log.String("topic", topic),
		log.Int32("partition", partition),
		log.Int64("previousOffset", previousOffset),
		log.Int64("offset", offset),
	)

	offsets := make(kadm.Offsets)
	offsets.AddOffset(topic, partition, offset, -1)
	responses, err := client.CommitOffsets(ctx, group, offsets)
	if err == nil {
		err = responses.Error()
	}
	if err != nil {
		// The broker only accepts commits without a member ID while the
		// group has no members, even if consumers joined after the check above.
		if errors.Is(err, kerr.UnknownMemberID) || errors.Is(err, kerr.IllegalGeneration) || errors.Is(err, kerr.RebalanceInProgress) {
			return nil, fmt.Errorf("unable to commit offset: consumer group %q has active members, scale its consumers to zero and retry: %w", group, err)
		}
		return nil, fmt.Errorf("unable to commit offset for consumer group %q: %w", group, err)
	}

	return &ResetConsumerGroupOffsetOutputs{
		Group:          group,
		Topic:          topic,
		Partition:      partition,
		PreviousOffset: previousOffset,
		Offset:         offset,
		LogStartOffset: logStartOffset,
		LogEndOffset:   logEndOffset,
	}, nil
}

func (inputs ResetConsumerGroupOffsetInputs) validate() error {
	switch {
	case inputs.Group == "":
		return errors.New("group is required")
	case inputs.Topic == "":
		return errors.New("topic is required")
	case inputs.Partition == nil:
		return errors.New("partition is required")
	case *inputs.Partition < 0:
		return fmt.Errorf("partition %d must not be negative", *inputs.Partition)
	case inputs.Offset == nil:
		return errors.New("offset is required")
	case *inputs.Offset < 0:
		return fmt.Errorf("offset %d must not be negative", *inputs.Offset)
	}
	return nil
}

// checkGroupIsInactive mirrors kafka-consumer-groups.sh, which only resets
// offsets of inactive groups. Unlike it, an unknown (Dead) group is rejected
// so that a mistyped group name does not create a new group.
func checkGroupIsInactive(group *ConsumerGroup) error {
	if strings.EqualFold(group.State, groupStateDead) {
		return fmt.Errorf("consumer group %q does not exist", group.Group)
	}
	if !strings.EqualFold(group.State, groupStateEmpty) || len(group.Members) > 0 {
		return fmt.Errorf("consumer group %q must be inactive to reset its offsets, but it is %s with %d member(s): scale its consumers to zero first", group.Group, group.State, len(group.Members))
	}
	return nil
}

func getPartitionBounds(ctx context.Context, client adminClient, topic string, partition int32) (int64, int64, error) {
	startOffsets, err := client.ListStartOffsets(ctx, topic)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list start offsets for topic %q: %w", topic, err)
	}
	endOffsets, err := client.ListEndOffsets(ctx, topic)
	if err != nil {
		return 0, 0, fmt.Errorf("unable to list end offsets for topic %q: %w", topic, err)
	}
	start, hasStart := startOffsets.Lookup(topic, partition)
	end, hasEnd := endOffsets.Lookup(topic, partition)
	if !hasStart || !hasEnd || errors.Is(start.Err, kerr.UnknownTopicOrPartition) || errors.Is(end.Err, kerr.UnknownTopicOrPartition) {
		return 0, 0, fmt.Errorf("topic %q partition %d does not exist", topic, partition)
	}
	if err := errors.Join(start.Err, end.Err); err != nil {
		return 0, 0, fmt.Errorf("unable to list offsets for topic %q partition %d: %w", topic, partition, err)
	}
	return start.Offset, end.Offset, nil
}
