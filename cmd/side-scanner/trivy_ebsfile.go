package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

type EBSClientWithWalk struct {
	*ebs.Client
}

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
