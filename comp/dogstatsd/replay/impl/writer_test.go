// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replayimpl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/benbjohnson/clock"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
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

	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	writer := NewTrafficCaptureWriter(1, taggerComponent, clock.New())

	// initialize telemeytry store
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	telemetryStore := packets.NewTelemetryStore(nil, telemetryComponent)

	// register pools

	manager := packets.NewPoolManager[packets.Packet](packets.NewPool(cfg, cfg.GetInt("dogstatsd_buffer_size"), telemetryStore))
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
				payload := "foo.bar|5|#some:tag"
				copy(pkt.Buffer, payload)
				pkt.Source = packets.UDS
				pkt.Contents = pkt.Buffer[:len(payload)]

				buff.Pb.Timestamp = time.Now().Unix()
				buff.Buff = pkt
				buff.Pb.Pid = 0
				buff.Pb.AncillarySize = int32(0)
				buff.Pb.PayloadSize = int32(len(payload))
				buff.Pb.Payload = pkt.Contents

				if writer.Enqueue(buff) {
					enqueued.Inc()
				}
				manager.Put(pkt)
			}

			writer.StopCapture()
		}(&wg, i)
	}

	session, err := writer.startCapture(file, testDuration, z)
	require.NoError(t, err)
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		close(start)
		<-session.done
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
	assert.Zero(t, manager.Count())
	assert.Zero(t, oobManager.Count())

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
	tagState, err := reader.ReadState()
	assert.Nil(t, tagState.PidMap)
	assert.Nil(t, tagState.State)
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

// testCaptureOutput is inspected only after session.done closes.
type testCaptureOutput struct {
	bytes.Buffer
	closed bool
}

func (w *testCaptureOutput) Close() error {
	w.closed = true
	return nil
}

type blockedCaptureOutput struct {
	testCaptureOutput
	entered chan struct{}
	proceed chan struct{}
	once    sync.Once
	fail    bool
}

func (w *blockedCaptureOutput) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.entered)
		<-w.proceed
	})
	if w.fail {
		return 0, errors.New("capture write failed")
	}
	return w.Buffer.Write(p)
}

func newTestCaptureWriter(t *testing.T, depth int) (*TrafficCaptureWriter, *packets.PoolManager[packets.Packet], *packets.PoolManager[[]byte], *clock.Mock) {
	t.Helper()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"telemetry.enabled":       false,
		"agent_telemetry.enabled": false,
	})
	manager := packets.NewPoolManager[packets.Packet](packets.NewPool(cfg, 8192, nil))
	oob := packets.NewPoolManager[[]byte](ddsync.NewSlicePool[byte](32, 32))
	clk := clock.NewMock()
	writer := NewTrafficCaptureWriter(depth, taggerfxmock.SetupFakeTagger(t), clk)
	require.NoError(t, writer.RegisterSharedPoolManager(manager))
	require.NoError(t, writer.RegisterOOBPoolManager(oob))
	return writer, manager, oob, clk
}

// advanceUntilDone drives the mock clock until the capture finishes. The capture
// goroutine registers its timer asynchronously, so a single Add can land first.
func advanceUntilDone(t *testing.T, clk *clock.Mock, step time.Duration, done <-chan struct{}) {
	t.Helper()
	for range 1000 {
		select {
		case <-done:
			return
		default:
		}
		clk.Add(step)
	}
	t.Fatal("capture did not finish after advancing the clock")
}

func captureTestBuffer(manager *packets.PoolManager[packets.Packet], oob *packets.PoolManager[[]byte], payload []byte) *replay.CaptureBuffer {
	pkt := manager.Get()
	copy(pkt.Buffer, payload)
	pkt.Contents = pkt.Buffer[:len(payload)]
	ancillary := oob.Get()
	copy(*ancillary, "credentials")
	return &replay.CaptureBuffer{
		Buff: pkt,
		Oob:  ancillary,
		Pb: replay.UnixDogstatsdMsg{
			Payload:       pkt.Contents,
			PayloadSize:   int32(len(payload)),
			Ancillary:     (*ancillary)[:11],
			AncillarySize: 11,
		},
	}
}

func waitCapture(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not finish")
	}
}

func TestWriterRepeatedCapture(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, depth := range []int{0, 2} {
			t.Run(fmt.Sprintf("compressed=%v/depth=%d", compressed, depth), func(t *testing.T) {
				writer, manager, oob, _ := newTestCaptureWriter(t, depth)
				for capture := range 3 {
					out := &testCaptureOutput{}
					session, err := writer.startCapture(out, time.Minute, compressed)
					require.NoError(t, err)
					payload := []byte(fmt.Sprintf("capture:%d|c", capture))
					msg := captureTestBuffer(manager, oob, payload)
					if capture == 0 {
						msg.Pid = 42
						msg.ContainerID = "container_id://first-capture"
					}
					require.True(t, writer.Enqueue(msg))
					manager.Put(msg.Buff)
					oob.Put(msg.Oob)
					writer.StopCapture()
					waitCapture(t, session.done)
					require.False(t, writer.IsOngoing())
					require.True(t, out.closed)
					require.Zero(t, manager.Count())
					require.Zero(t, oob.Count())
					data := out.Bytes()
					if compressed {
						data, err = zstd.Decompress(nil, data)
						require.NoError(t, err)
					}
					reader := &TrafficCaptureReader{Contents: data, Version: int(datadogFileVersion)}
					reader.Seek(0)
					recorded, err := reader.ReadNext()
					require.NoError(t, err)
					require.Equal(t, payload, recorded.Payload)
					require.Equal(t, []byte("credentials"), recorded.Ancillary)
					_, err = reader.ReadNext()
					require.ErrorIs(t, err, io.EOF)
					state, err := reader.ReadState()
					require.NoError(t, err)
					if capture == 0 {
						require.Equal(t, "container_id://first-capture", state.PidMap[42])
					} else {
						require.Empty(t, state.PidMap, "tagger state must not cross sessions")
					}
					// Normal traffic must keep recycling after every capture.
					for range 128 {
						manager.Put(manager.Get())
						oob.Put(oob.Get())
					}
					require.Zero(t, manager.Count())
					require.Zero(t, oob.Count())
				}
			})
		}
	}
}

func TestWriterConcurrentStart(t *testing.T) {
	writer, _, _, _ := newTestCaptureWriter(t, 0)
	start := make(chan struct{})
	accepted := make(chan *captureSession, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out := &testCaptureOutput{}
			if s, err := writer.startCapture(out, time.Minute, false); err == nil {
				accepted <- s
			} else {
				out.Close() // the rejected caller still owns its target
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Len(t, accepted, 1)
	first := <-accepted
	writer.StopCapture()
	waitCapture(t, first.done)

	second, err := writer.startCapture(&testCaptureOutput{}, time.Minute, false)
	require.NoError(t, err)
	writer.stopSession(first) // simulate a late timer callback from the old session
	select {
	case <-second.stop:
		t.Fatal("old session stopped a newer capture")
	default:
	}
	writer.StopCapture()
	writer.StopCapture()
	waitCapture(t, second.done)
}

func TestWriterStopUnblocksEnqueue(t *testing.T) {
	for _, fail := range []bool{false, true} {
		for _, depth := range []int{0, 1} {
			t.Run(fmt.Sprintf("writeError=%v/depth=%d", fail, depth), func(t *testing.T) {
				writer, manager, oob, _ := newTestCaptureWriter(t, depth)
				component := &trafficCapture{writer: writer}
				out := &blockedCaptureOutput{entered: make(chan struct{}), proceed: make(chan struct{}), fail: fail}
				var unblock sync.Once
				releaseWrite := func() { unblock.Do(func() { close(out.proceed) }) }
				defer releaseWrite()
				session, err := writer.startCapture(out, time.Minute, false)
				require.NoError(t, err)
				defer writer.StopCapture()
				first := captureTestBuffer(manager, oob, bytes.Repeat([]byte("x"), 8000))
				require.True(t, component.Enqueue(first))
				manager.Put(first.Buff)
				oob.Put(first.Oob)
				waitCapture(t, out.entered) // the consumer is now blocked in Write
				if depth > 0 {
					queued := captureTestBuffer(manager, oob, []byte("queued:1|c"))
					require.True(t, component.Enqueue(queued))
					manager.Put(queued.Buff)
					oob.Put(queued.Oob)
				}
				pending := captureTestBuffer(manager, oob, []byte("pending:1|c"))
				result := make(chan bool, 1)
				go func() {
					accepted := component.Enqueue(pending)
					manager.Put(pending.Buff)
					oob.Put(pending.Oob)
					result <- accepted
				}()
				require.Eventually(t, func() bool { return oob.Count() == depth+2 }, time.Second, time.Millisecond)
				if fail {
					releaseWrite() // consumer discovers the error and must wake senders itself
				} else {
					stopped := make(chan struct{})
					go func() { component.StopCapture(); close(stopped) }()
					waitCapture(t, stopped)
				}
				select {
				case accepted := <-result:
					require.False(t, accepted)
				case <-time.After(time.Second):
					t.Fatal("blocked enqueue did not wake on stop")
				}
				if !fail {
					require.Equal(t, depth+1, manager.Count(), "queued and writing buffers must remain retained")
					require.Equal(t, depth+1, oob.Count())
					require.True(t, writer.IsOngoing(), "must not admit another capture during drain")
					releaseWrite()
				}
				waitCapture(t, session.done)
				require.True(t, out.closed)
				require.False(t, writer.IsOngoing())
				require.Zero(t, manager.Count())
				require.Zero(t, oob.Count())
				require.False(t, component.Enqueue(pending))
			})
		}
	}
}

func TestWriterProcessingOutlivesCapture(t *testing.T) {
	writer, manager, oob, _ := newTestCaptureWriter(t, 0)
	first, err := writer.startCapture(&testCaptureOutput{}, time.Minute, false)
	require.NoError(t, err)
	msg := captureTestBuffer(manager, oob, []byte("first:1|c"))
	require.True(t, writer.Enqueue(msg))
	writer.StopCapture()
	waitCapture(t, first.done)
	require.Equal(t, 1, manager.Count(), "normal processing still owns this packet")
	require.Equal(t, 1, oob.Count())

	second, err := writer.startCapture(&testCaptureOutput{}, time.Minute, false)
	require.NoError(t, err)
	other := captureTestBuffer(manager, oob, []byte("second:1|c"))
	require.NotSame(t, msg.Buff, other.Buff)
	require.NotSame(t, msg.Oob, other.Oob)
	require.True(t, writer.Enqueue(other))
	manager.Put(other.Buff)
	oob.Put(other.Oob)
	writer.StopCapture()
	waitCapture(t, second.done)
	require.Equal(t, 1, manager.Count())
	manager.Put(msg.Buff)
	oob.Put(msg.Oob)
	require.Zero(t, manager.Count())
	require.Zero(t, oob.Count())
}

func TestWriterCaptureTimeout(t *testing.T) {
	writer, manager, oob, clk := newTestCaptureWriter(t, 0)
	for range 2 {
		session, err := writer.startCapture(&testCaptureOutput{}, 10*time.Millisecond, false)
		require.NoError(t, err)
		// Only the duration timer can end this capture.
		require.True(t, writer.IsOngoing())
		advanceUntilDone(t, clk, 10*time.Millisecond, session.done)
		require.False(t, writer.IsOngoing())
		require.Zero(t, manager.Count())
		require.Zero(t, oob.Count())
	}
}

// TestWriterStopAndWait: shutdown must not return before the capture has
// drained and flushed.
func TestWriterStopAndWait(t *testing.T) {
	writer, manager, oob, _ := newTestCaptureWriter(t, 2)
	out := &testCaptureOutput{}
	session, err := writer.startCapture(out, time.Minute, false)
	require.NoError(t, err)
	msg := captureTestBuffer(manager, oob, []byte("shutdown:1|c"))
	require.True(t, writer.Enqueue(msg))
	manager.Put(msg.Buff)
	oob.Put(msg.Oob)

	writer.StopAndWait(5 * time.Second)

	select {
	case <-session.done:
	default:
		t.Fatal("StopAndWait returned before the capture finished")
	}
	require.False(t, writer.IsOngoing())
	require.True(t, out.closed)
	require.Zero(t, manager.Count())
	require.Zero(t, oob.Count())

	reader := &TrafficCaptureReader{Contents: out.Bytes(), Version: int(datadogFileVersion)}
	reader.Seek(0)
	recorded, err := reader.ReadNext()
	require.NoError(t, err)
	require.Equal(t, []byte("shutdown:1|c"), recorded.Payload)
	_, err = reader.ReadState()
	require.NoError(t, err, "a drained capture must end with a parseable state trailer")

	// Idempotent, and safe with no session.
	writer.StopAndWait(5 * time.Second)
}

func TestWriterFlushError(t *testing.T) {
	writer, manager, oob, _ := newTestCaptureWriter(t, 0)
	out := &blockedCaptureOutput{entered: make(chan struct{}), proceed: make(chan struct{}), fail: true}
	close(out.proceed)
	session, err := writer.startCapture(out, time.Minute, false)
	require.NoError(t, err)
	msg := captureTestBuffer(manager, oob, []byte("buffered:1|c"))
	require.True(t, writer.Enqueue(msg))
	manager.Put(msg.Buff)
	oob.Put(msg.Oob)
	writer.StopCapture()
	waitCapture(t, session.done)
	require.True(t, out.closed)
	require.Zero(t, manager.Count())
	require.Zero(t, oob.Count())
	next, err := writer.startCapture(&testCaptureOutput{}, time.Minute, false)
	require.NoError(t, err, "flush errors must not prevent subsequent captures")
	writer.StopCapture()
	waitCapture(t, next.done)
}
