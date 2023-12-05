// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

// EBSClientWithWalk represents an EBS client with walker.
type EBSClientWithWalk struct {
	*ebs.Client
}

// WalkSnapshotBlocks method walks though snapshot blocks.
func (e EBSClientWithWalk) WalkSnapshotBlocks(ctx context.Context, input *ebs.ListSnapshotBlocksInput, table map[int32]string) (*ebs.ListSnapshotBlocksOutput, map[int32]string, error) {
	for {
		output, err := e.ListSnapshotBlocks(ctx, input)
		if err != nil {
			return nil, nil, err
		}
		for _, block := range output.Blocks {
			table[*block.BlockIndex] = *block.BlockToken
		}
		output.VolumeSize = aws.Int64(*output.VolumeSize << 30)
		if output.NextToken == nil {
			return output, table, nil
		}
		input.NextToken = output.NextToken
	}
}
