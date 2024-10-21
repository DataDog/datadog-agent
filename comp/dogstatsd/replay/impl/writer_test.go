// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replayimpl

import (
	"io"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

func writerTest(t *testing.T, z bool) {
	fs := afero.NewMemMapFs()
	// setup directory
	fs.MkdirAll("foo/bar", 0777)
	file, path, err := OpenFile(fs, "foo/bar", "")
	require.NoError(t, err)

	cfg := config.NewMock(t)

	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())

	writer := NewTrafficCaptureWriter(1, taggerComponent)

	// initialize telemeytry store
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	telemetryStore := packets.NewTelemetryStore(nil, telemetryComponent)

	// register pools

	manager := packets.NewPoolManager[packets.Packet](packets.NewPool(cfg.GetInt("dogstatsd_buffer_size"), telemetryStore))
	oobManager := packets.NewPoolManager[[]byte](ddsync.NewSlicePool[byte](32, 32))

	require.NoError(t, writer.RegisterSharedPoolManager(manager))
	require.NoError(t, writer.RegisterOOBPoolManager(oobManager))

	var wg sync.WaitGroup
	const (
		iterations   = 100
		testDuration = 5 * time.Second
	)
	sleepDuration := testDuration / iterations
	// For test to fail consistently we need to run with more threads than available CPU
	threads := runtime.NumCPU()
	start := make(chan struct{})
	enqueued := atomic.NewInt32(0)

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup, threadNo int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(9223372036854 / (threadNo + 1))))
			<-start
			// Add a little bit of controlled jitter so tests fail if enqueuing not correct
			duration := time.Duration(r.Int63n(int64(sleepDuration)))
			time.Sleep(duration)

			for i := 0; i < iterations; i++ {
				buff := new(replay.CaptureBuffer)
				pkt := manager.Get()
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
					enqueued.Inc()
				}
			}

			writer.StopCapture()
		}(&wg, i)
	}

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		close(start)
		writer.Capture(file, testDuration, z)
	}(&wg)

	wgc := make(chan struct{})
	go func(wg *sync.WaitGroup) {
		defer close(wgc)
		wg.Wait()
	}(&wg)

	<-start
	select {
	case <-wgc:
		break
	case <-time.After(testDuration * 2):
		assert.FailNow(t, "Timed out waiting for capture to finish", "Timeout was: %v", testDuration*2)
	}

	// assert file
	assert.False(t, writer.IsOngoing())

	stats, _ := file.Stat()
	assert.Greater(t, stats.Size(), int64(0))

	var (
		buf    []byte
		reader *TrafficCaptureReader
	)

	fp, err := fs.Open(path)
	assert.Nil(t, err)
	buf, err = afero.ReadAll(fp)
	assert.Nil(t, err)

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

	var cnt int32
	for _, err = reader.ReadNext(); err != io.EOF; _, err = reader.ReadNext() {
		cnt++
	}
	assert.Equal(t, cnt, enqueued.Load())
}

func TestWriterUncompressed(t *testing.T) {
	writerTest(t, false)
}

func TestWriterCompressed(t *testing.T) {
	writerTest(t, true)
}

func TestValidateLocation(t *testing.T) {
	fs := afero.NewMemMapFs()

	locationBad := "foo/bar"
	locationGood := "bar/quz"

	// setup directory
	fs.MkdirAll(locationBad, 0770)
	fs.MkdirAll(locationGood, 0776)

	_, err := validateLocation(fs, locationBad, "")
	assert.NotNil(t, err)
	l, err := validateLocation(fs, locationGood, "")
	assert.Nil(t, err)
	assert.Equal(t, locationGood, l)
}
