// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/jinroh/go-nbd/pkg/backend"
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

// EBSBlockDeviceOptions represents the options of the EBS block device.
type EBSBlockDeviceOptions struct {
	EBSClient   *ebs.Client
	DeviceName  string
	SnapshotARN arn.ARN
	RunClient   bool
}

type EBSBlockDevice struct {
	EBSBlockDeviceOptions
	wg sync.WaitGroup
}

// NewEBSBlockDevice sets up the EBS block device.
func NewEBSBlockDevice(opts EBSBlockDeviceOptions) EBSBlockDevice {
	return EBSBlockDevice{
		EBSBlockDeviceOptions: opts,
	}
}

func (bd *EBSBlockDevice) Start(ctx context.Context) error {
	_, err := os.Stat(bd.DeviceName)
	if err != nil {
		return fmt.Errorf("ebsblockdevice: could not stat device %q: %w", bd.DeviceName, err)
	}

	ready := make(chan error)
	bd.wg.Add(1)
	go func() {
		bd.startServer(ctx, ready)
		bd.wg.Done()
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-ready:
		if err != nil {
			return err
		}
	}

	if bd.RunClient {
		bd.wg.Add(1)
		if err := bd.startClient(ctx); err != nil {
			bd.wg.Done()
			return err
		}
		go func() {
			<-ctx.Done()
			bd.closeClient()
			bd.wg.Done()
		}()
	}
	return nil
}

func (bd *EBSBlockDevice) WaitCleanup() {
	bd.wg.Wait()
}

func (bd *EBSBlockDevice) getSocketAddr(device string, snapshotARN arn.ARN) string {
	snapshotID := strings.TrimPrefix(snapshotARN.Resource, "snapshot/")
	return fmt.Sprintf("/tmp/nbd-ebs-%s-%s", path.Base(device), snapshotID)
}

func (bd *EBSBlockDevice) startClient(ctx context.Context) error {
	_, err := exec.LookPath("nbd-client")
	if err != nil {
		return fmt.Errorf("ebsblockdevice: could not locate 'nbd-client' util binary in PATH: %w", err)
	}
	addr := bd.getSocketAddr(bd.DeviceName, bd.SnapshotARN)
	cmd := exec.CommandContext(ctx, "nbd-client",
		"-unix", addr, bd.DeviceName,
		"-name", bd.SnapshotARN.Resource,
		"-connections", "5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("nbd-client: failed: %s", string(out))
		return err
	}
	return nil
}

func (bd *EBSBlockDevice) closeClient() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Debugf("nbdclient: destroying client for device %q", bd.DeviceName)
	if err := exec.CommandContext(ctx, "nbd-client", "-d", bd.DeviceName).Run(); err != nil {
		log.Errorf("nbd-client: disconnecting %q failed: %v", bd.DeviceName, err)
	} else {
		log.Debugf("nbd-client: disconnected device %q", bd.DeviceName)
	}
}

func (bd *EBSBlockDevice) startServer(ctx context.Context, ready chan<- error) {
	var lc net.ListenConfig
	_, snapshotID, _ := getARNResource(bd.SnapshotARN)
	b, err := newEBSBackend(ctx, bd.EBSClient, snapshotID)
	if err != nil {
		ready <- fmt.Errorf("ebsblockdevice: could not start backend: %w", err)
		return
	}

	addr := bd.getSocketAddr(bd.DeviceName, bd.SnapshotARN)
	if _, err := os.Stat(addr); err == nil {
		if err := os.Remove(addr); err != nil {
			ready <- fmt.Errorf("ebsblockdevice: could not delete %q: %w", addr, err)
			return
		}
	}

	srv, err := lc.Listen(ctx, "unix", addr)
	if err != nil {
		ready <- fmt.Errorf("ebsblockdevice: could not list to %q: %w", addr, err)
		return
	}
	if err := os.Chmod(addr, 700); err != nil {
		ready <- fmt.Errorf("ebsblockdevice: could not chmod %q: %w", addr, err)
		return
	}

	conns := make(map[net.Conn]struct{})
	addConn := make(chan net.Conn)
	rmvConn := make(chan net.Conn)

	go func() {
		for {
			conn, err := srv.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				log.Warnf("nbdserver: could not accept connection: %v", err)
			} else {
				addConn <- conn
			}
		}
	}()

	log.Infof("nbd server accepting connections on %q", addr)
	ready <- nil
	for {
		select {
		case conn := <-addConn:
			conns[conn] = struct{}{}
			go func() {
				bd.serverHandleConn(conn, b)
				log.Debugf("nbdserver: client disconnected")
				rmvConn <- conn
			}()

		case conn := <-rmvConn:
			delete(conns, conn)
			conn.Close()

		case <-ctx.Done():
			log.Debugf("nbdserver: closing server for device %q", bd.DeviceName)
			for conn := range conns {
				conn.Close()
			}
			if err := srv.Close(); err != nil {
				log.Errorf("nbdserver: could not close server: %v", err)
			}
			return
		}
	}
}

func (bd *EBSBlockDevice) serverHandleConn(conn net.Conn, backend backend.Backend) {
	log.Debugf("nbdserver: client connected %q", conn.RemoteAddr())
	err := server.Handle(conn,
		[]*server.Export{
			{
				Name:    bd.SnapshotARN.Resource,
				Backend: backend,
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
		log.Errorf("nbdserver: could not handle new connection %q: %v", conn.RemoteAddr(), err)
	}
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

func (b *ebsBackend) WriteAt(_ []byte, _ int64) (n int, err error) {
	panic("ebsblockdevice: read-only file system")
}

func (b *ebsBackend) Size() (int64, error) {
	return b.size, nil
}

func (b *ebsBackend) Sync() error {
	return nil
}
