//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/jinroh/go-nbd/pkg/client"
	"github.com/jinroh/go-nbd/pkg/server"
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

type EBSBlockDeviceOptions struct {
	EBSClient   *ebs.Client
	Name        string
	DeviceName  string
	Description string
	SnapshotARN arn.ARN
}

func SetupEBSBlockDevice(ctx context.Context, opts EBSBlockDeviceOptions) error {
	_, err := os.Stat(opts.DeviceName)
	if err != nil {
		return fmt.Errorf("ebsblockdevice: could not stat device %q: %w", opts.DeviceName, err)
	}

	ready := make(chan error)
	go startServer(ctx, opts, ready)
	select {
	case <-ctx.Done():
		return nil
	case err := <-ready:
		if err != nil {
			return err
		}
	}
	go func() {
		ready <- startClient(ctx, opts)
	}()
	select {
	// TODO: better polling to check for the setup readiness
	case <-time.After(3 * time.Second):
		return nil
	case err := <-ready:
		return err
	}
}

func getSocketAddr(device string, snapshotARN arn.ARN) string {
	h := sha256.New()
	h.Write([]byte(snapshotARN.String()))
	return fmt.Sprintf("/tmp/nbd-ebs-%s-%x", path.Base(device), h.Sum(nil))
}

func startClient(ctx context.Context, opts EBSBlockDeviceOptions) error {
	dev, err := os.Open(opts.DeviceName)
	if err != nil {
		return fmt.Errorf("ebsblockdevice: could not open device %q: %w", opts.DeviceName, err)
	}
	defer dev.Close()

	var d net.Dialer
	addr := getSocketAddr(opts.DeviceName, opts.SnapshotARN)
	conn, err := d.DialContext(ctx, "unix", addr)
	if err != nil {
		return fmt.Errorf("ebsblockdevice: could not dial %s: %v", addr)
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		log.Debugf("nbdclient: disconnecting %s", dev.Name())
		if err := client.Disconnect(dev); err != nil {
			log.Warnf("nbdclient: disconnected with error %s: %v", dev.Name(), err)
		}
	}()

	err = client.Connect(conn, dev, &client.Options{
		ExportName: opts.Name,
		BlockSize:  512,
	})
	if err != nil {
		log.Errorf("nbdclient: could not start: %v", err)
	}
	log.Debugf("nbdclient: finished %s: %v", dev.Name(), err)
	return err
}

func startServer(ctx context.Context, opts EBSBlockDeviceOptions, ready chan error) {
	var lc net.ListenConfig
	_, snapshotID, _ := getARNResource(opts.SnapshotARN)
	b, err := newEBSBackend(ctx, opts.EBSClient, snapshotID)
	if err != nil {
		ready <- fmt.Errorf("ebsblockdevice: could not start backend: %w", err)
		return
	}

	addr := getSocketAddr(opts.DeviceName, opts.SnapshotARN)
	if _, err := os.Stat(addr); err == nil {
		os.Remove(addr)
	}
	l, err := lc.Listen(ctx, "unix", addr)
	if err != nil {
		ready <- fmt.Errorf("ebsblockdevice: could not list to %q: %w", addr, err)
		return
	}
	defer l.Close()
	defer os.Remove(addr)

	go func() {
		ready <- nil
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				log.Warnf("nbdserver: could not accept connection: %v", err)
				continue
			}

			go func() {
				defer func() {
					log.Debugf("nbdserver: client disconnected")
					conn.Close()
				}()

				log.Debugf("nbdserver: client connected %q", conn.LocalAddr())
				err := server.Handle(conn,
					[]*server.Export{
						{
							Name:        opts.Name,
							Description: opts.Description,
							Backend:     b,
						},
					},
					&server.Options{
						ReadOnly:           true,
						MinimumBlockSize:   1,
						PreferredBlockSize: 4096,
						MaximumBlockSize:   0xffffffff,
						SupportsMultiConn:  true,
					})
				if err != nil {
					log.Errorf("nbdserver: could not handle new connection %q: %v", conn.LocalAddr(), err)
				}
			}()
		}
	}()

	<-ctx.Done()
}

type ebsBackend struct {
	ctx        context.Context
	ebsclient  *ebs.Client
	snapshotID string

	cache   *lru.Cache[int32, []byte]
	cacheMu sync.RWMutex

	singlegroup *singleflight.Group

	index map[int32]string
	size  int64
	lock  sync.Mutex
}

func newEBSBackend(ctx context.Context, ebsclient *ebs.Client, snapshotID string) (*ebsBackend, error) {
	if snapshotID == "" {
		return nil, fmt.Errorf("ebsblockdevice: missing snapshotID")
	}

	cache, err := lru.NewWithEvict[int32, []byte](ebsCacheSize, func(_ int32, block []byte) {
		blockPool.Put(block)
	})
	if err != nil {
		panic(err)
	}
	b := &ebsBackend{
		ctx:         ctx,
		ebsclient:   ebsclient,
		snapshotID:  snapshotID,
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
		log.Tracef("copy block=%d start=%d len=%d\n", blockIndex, copyStart, copyMax)
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
	blockOutput, err := b.ebsclient.GetSnapshotBlock(b.ctx, &ebs.GetSnapshotBlockInput{
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
		output, err := b.ebsclient.ListSnapshotBlocks(b.ctx, &ebs.ListSnapshotBlocksInput{
			SnapshotId: &b.snapshotID,
			NextToken:  nextToken,
		})
		if err != nil {
			return err
		}
		log.Debugf("list blocks %d\n", len(output.Blocks))
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

func (b *ebsBackend) WriteAt(p []byte, off int64) (n int, err error) {
	panic("ebsblockdevice: read-only file system")
}

func (b *ebsBackend) Size() (int64, error) {
	return b.size, nil
}

func (b *ebsBackend) Sync() error {
	return nil
}
