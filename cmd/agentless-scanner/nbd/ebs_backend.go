// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package nbd

import (
	"context"
	"io"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/jinroh/go-nbd/pkg/backend"
)

const (
	ebsBlockSize = 512 * 1024
	ebsCacheSize = 128
)

var (
	nullBlock = make([]byte, ebsBlockSize)
	blockPool = sync.Pool{
		New: func() any {
			return make([]byte, ebsBlockSize)
		},
	}
)

type ebsBackend struct {
	ebsclient  *ebs.Client
	snapshotID string

	cache   *lru.Cache[int32, []byte]
	cacheMu sync.RWMutex

	singlegroup *singleflight.Group

	index map[int32]string
	size  int64
}

// NewEBSBackend creates a new EBS NBD backend for the given snapshot ID.
func NewEBSBackend(ebsclient *ebs.Client, snapshot types.CloudID) (backend.Backend, error) {
	cache, err := lru.NewWithEvict[int32, []byte](ebsCacheSize, func(_ int32, block []byte) {
		blockPool.Put(block)
	})
	if err != nil {
		panic(err)
	}
	b := &ebsBackend{
		ebsclient:   ebsclient,
		snapshotID:  snapshot.ResourceName(),
		cache:       cache,
		singlegroup: new(singleflight.Group),
	}
	if err := b.init(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *ebsBackend) ReadAt(p []byte, off int64) (n int, err error) {
	for len(p) > 0 {
		blockIndex := int32(off / ebsBlockSize)
		block, err := b.readBlock(blockIndex)
		if err != nil {
			return n, err
		}
		copyMax := int64(len(p))
		copyStart := off % ebsBlockSize
		copyEnd := copyStart + copyMax
		if copyEnd > ebsBlockSize {
			copyEnd = ebsBlockSize
		}
		copied := copy(p, block[copyStart:copyEnd])
		off += int64(copied)
		p = p[copied:]
		n += copied
		if off > b.size {
			n -= int(b.size - off)
			return n, io.EOF
		}
	}
	return n, nil
}

func (b *ebsBackend) readBlock(blockIndex int32) ([]byte, error) {
	blockToken, ok := b.index[blockIndex]
	if !ok {
		return nullBlock, nil
	}
	b.cacheMu.RLock()
	if block, ok := b.cache.Get(blockIndex); ok {
		b.cacheMu.RUnlock()
		return block, nil
	}
	b.cacheMu.RUnlock()
	bl, err, _ := b.singlegroup.Do(strconv.FormatInt(int64(blockIndex), 10), func() (interface{}, error) {
		block, err := b.fetchBlock(blockIndex, blockToken)
		if err != nil {
			return nil, err
		}
		b.cacheMu.Lock()
		b.cache.Add(blockIndex, block)
		b.cacheMu.Unlock()
		return block, nil
	})
	if err != nil {
		return nil, err
	}
	return bl.([]byte), nil
}

func (b *ebsBackend) fetchBlock(blockIndex int32, blockToken string) ([]byte, error) {
	log.Tracef("fetching block %d", blockIndex)
	blockOutput, err := b.ebsclient.GetSnapshotBlock(context.Background(), &ebs.GetSnapshotBlockInput{
		SnapshotId: aws.String(b.snapshotID),
		BlockIndex: aws.Int32(int32(blockIndex)),
		BlockToken: aws.String(blockToken),
	})
	if err != nil {
		return nil, err
	}
	block := blockPool.Get().([]byte)
	defer blockOutput.BlockData.Close()
	_, err = io.ReadFull(blockOutput.BlockData, block)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (b *ebsBackend) init() error {
	var nextToken *string
	for {
		output, err := b.ebsclient.ListSnapshotBlocks(context.Background(), &ebs.ListSnapshotBlocksInput{
			SnapshotId: &b.snapshotID,
			NextToken:  nextToken,
		})
		if err != nil {
			return err
		}
		log.Tracef("list blocks %d\n", len(output.Blocks))
		if b.index == nil {
			b.index = make(map[int32]string)
		}
		if *output.BlockSize != ebsBlockSize {
			panic("unexpected block size")
		}
		for _, block := range output.Blocks {
			b.index[*block.BlockIndex] = *block.BlockToken
		}
		nextToken = output.NextToken
		if nextToken == nil {
			b.size = *output.VolumeSize * 1024 * 1024 * 1024
			return nil
		}
	}
}

func (b *ebsBackend) WriteAt(_ []byte, _ int64) (n int, err error) {
	panic("ebsblockdevice: read-only file system")
}

func (b *ebsBackend) Size() (int64, error) {
	return b.size, nil
}

func (b *ebsBackend) Sync() error {
	return nil
}
