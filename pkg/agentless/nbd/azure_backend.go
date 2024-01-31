// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package nbd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/jinroh/go-nbd/pkg/backend"
)

const (
	// Unlike on AWS, on Azure we can tune the block size.
	// However, for now we keep it the same as on AWS.
	absBlockSize            = ebsBlockSize
	absCacheSize            = ebsCacheSize
	absSASDurationInSeconds = 3600
)

var (
	azureBlockPool = sync.Pool{
		New: func() any {
			return make([]byte, absBlockSize)
		},
	}
)

type azureBackend struct {
	snapshotID      *arm.ResourceID
	snapshotsClient *armcompute.SnapshotsClient
	sasClient       *http.Client
	accessURI       *armcompute.AccessURI

	cache   *lru.Cache[int32, []byte]
	cacheMu sync.RWMutex

	singlegroup *singleflight.Group

	size int64
}

// NewAzureBackend creates a new Azure NBD backend for the given snapshot.
func NewAzureBackend(snapshotsClient *armcompute.SnapshotsClient, snapshot *armcompute.Snapshot) (backend.Backend, error) {
	snapshotID, err := arm.ParseResourceID(*snapshot.ID)
	if err != nil {
		return nil, err
	}

	cache, err := lru.NewWithEvict[int32, []byte](absCacheSize, func(_ int32, block []byte) {
		azureBlockPool.Put(block)
	})
	if err != nil {
		panic(err)
	}
	b := &azureBackend{
		snapshotID:      snapshotID,
		snapshotsClient: snapshotsClient,
		sasClient:       &http.Client{},
		cache:           cache,
		singlegroup:     new(singleflight.Group),
		size:            *snapshot.Properties.DiskSizeBytes,
	}

	log.Tracef("getting SAS URI for snapshot %s", snapshotID)
	b.accessURI, err = b.getAccessURI(context.Background())

	if err != nil {
		return nil, err
	}
	return b, nil
}

func (b *azureBackend) ReadAt(p []byte, off int64) (n int, err error) {
	for len(p) > 0 {
		blockIndex := int32(off / absBlockSize)
		block, err := b.readBlock(blockIndex)
		if err != nil {
			return n, err
		}
		copyMax := int64(len(p))
		copyStart := off % absBlockSize
		copyEnd := copyStart + copyMax
		if copyEnd > absBlockSize {
			copyEnd = absBlockSize
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

func (b *azureBackend) readBlock(blockIndex int32) ([]byte, error) {
	if int64(blockIndex+1)*int64(absBlockSize) > b.size {
		return nil, fmt.Errorf("azure_backend: block index out of bounds: %d", blockIndex)
	}

	b.cacheMu.RLock()
	if block, ok := b.cache.Get(blockIndex); ok {
		b.cacheMu.RUnlock()
		return block, nil
	}
	b.cacheMu.RUnlock()
	bl, err, _ := b.singlegroup.Do(strconv.FormatInt(int64(blockIndex), 10), func() (interface{}, error) {
		block, err := b.fetchBlock(blockIndex)
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

func (b *azureBackend) fetchBlock(blockIndex int32) (block []byte, err error) {
	log.Tracef("fetching block %d", blockIndex)

	block = azureBlockPool.Get().([]byte)
	_, err = b.fetchRange(block, int64(blockIndex)*int64(absBlockSize))

	return block, err
}

func (b *azureBackend) fetchRange(p []byte, off int64) (n int, err error) {
	bytes := fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p)))
	log.Tracef("fetching range %s", bytes)

	req, _ := http.NewRequest("GET", *b.accessURI.AccessSAS, nil)
	req.Header.Add("Range", bytes)

	resp, err := b.sasClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return io.ReadFull(resp.Body, p)
}

func (b *azureBackend) getAccessURI(ctx context.Context) (*armcompute.AccessURI, error) {
	poller, err := b.snapshotsClient.BeginGrantAccess(ctx, b.snapshotID.ResourceGroupName, b.snapshotID.Name,
		armcompute.GrantAccessData{
			Access:            to.Ptr(armcompute.AccessLevelRead),
			DurationInSeconds: to.Ptr[int32](absSASDurationInSeconds),
		},
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.AccessURI, nil
}

func (b *azureBackend) WriteAt([]byte, int64) (int, error) {
	return 0, fmt.Errorf("azure_backend: read-only file system")
}

func (b *azureBackend) Size() (int64, error) {
	return b.size, nil
}

func (b *azureBackend) Sync() error {
	return nil
}
