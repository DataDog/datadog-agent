// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"io"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/zstd"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func writerTest(t *testing.T, z bool) {
	captureFs.Lock()
	originalFs := captureFs.fs
	captureFs.fs = afero.NewMemMapFs()
	captureFs.Unlock()

	// setup directory
	captureFs.fs.MkdirAll("foo/bar", 0777)

	defer func() {
		captureFs.Lock()
		defer captureFs.Unlock()

		captureFs.fs = originalFs
	}()

	writer := NewTrafficCaptureWriter(1)

	// register pools
	manager := packets.NewPoolManager(packets.NewPool(config.Datadog.GetInt("dogstatsd_buffer_size")))
	oobManager := packets.NewPoolManager(packets.NewPool(32))

	writer.RegisterSharedPoolManager(manager)
	writer.RegisterOOBPoolManager(oobManager)

	var wg sync.WaitGroup
	const (
		iterations    = 50
		sleepInterval = 100 * time.Millisecond
	)

	start := make(chan struct{})

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		close(start)
		writer.Capture("foo/bar", iterations*sleepInterval, z)
	}(&wg)

	enqueued := 0
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		<-start

		for i := 0; i < iterations; i++ {
			time.Sleep(sleepInterval)
			buff := CapPool.Get().(*CaptureBuffer)
			pkt := manager.Get().(*packets.Packet)
			pkt.Buffer = []byte("foo.bar|5|#some:tag")
			pkt.Source = packets.UDS
			pkt.Contents = pkt.Buffer

			buff.Pb.Timestamp = time.Now().Unix()
			buff.Buff = pkt
			buff.Pb.Pid = 0
			buff.Pb.AncillarySize = int32(0)
			buff.Pb.PayloadSize = int32(len(pkt.Buffer))
			buff.Pb.Payload = pkt.Buffer // or packet.Buffer[:n] ?

			if writer.Enqueue(buff) {
				enqueued++
			}
		}

		writer.StopCapture()
	}(&wg)

	wg.Wait()

	// assert file
	writer.RLock()
	assert.NotNil(t, writer.File)
	assert.False(t, writer.ongoing)

	stats, _ := writer.File.Stat()
	assert.Greater(t, stats.Size(), int64(0))

	var (
		err    error
		buf    []byte
		reader *TrafficCaptureReader
	)

	info, err := writer.File.Stat()
	assert.Nil(t, err)
	fp, err := captureFs.fs.Open(path.Join(writer.Location, info.Name()))
	assert.Nil(t, err)
	buf, err = afero.ReadAll(fp)
	assert.Nil(t, err)
	writer.RUnlock()

	if z {
		buf, err = zstd.Decompress(nil, buf)
		assert.Nil(t, err)
	}

	reader = &TrafficCaptureReader{
		Contents: buf,
		Version:  int(datadogFileVersion),
		Traffic:  make(chan *pb.UnixDogstatsdMsg, 1),
	}

	// file should contain no state as traffic had no ancillary data
	pidMap, entityMap, err := reader.ReadState()
	assert.Nil(t, pidMap)
	assert.Nil(t, entityMap)
	assert.Nil(t, err)

	reader.Lock()
	reader.offset = uint32(len(datadogHeader))
	reader.Unlock()

	cnt := 0
	for _, err = reader.ReadNext(); err != io.EOF; _, err = reader.ReadNext() {
		cnt++
	}
	assert.Equal(t, cnt, enqueued)
}

func TestWriterUncompressed(t *testing.T) {
	writerTest(t, false)
}

func TestWriterCompressed(t *testing.T) {
	writerTest(t, true)
}

func TestValidateLocation(t *testing.T) {
	captureFs.Lock()
	originalFs := captureFs.fs
	captureFs.fs = afero.NewMemMapFs()
	captureFs.Unlock()

	locationBad := "foo/bar"
	locationGood := "bar/quz"

	// setup directory
	captureFs.fs.MkdirAll(locationBad, 0770)
	captureFs.fs.MkdirAll(locationGood, 0776)

	defer func() {
		captureFs.Lock()
		defer captureFs.Unlock()

		captureFs.fs = originalFs
	}()

	writer := NewTrafficCaptureWriter(1)
	_, err := writer.ValidateLocation(locationBad)
	assert.NotNil(t, err)
	l, err := writer.ValidateLocation(locationGood)
	assert.Nil(t, err)
	assert.Equal(t, locationGood, l)

}
