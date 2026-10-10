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
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/zstd"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

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

// Build the Fx fixture outside synctest: its background goroutines use global
// channels and must not become part of a test bubble.
func newTestCaptureWriter(t *testing.T, depth int) (*TrafficCaptureWriter, *packets.PoolManager[packets.Packet], *packets.PoolManager[[]byte]) {
	t.Helper()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"telemetry.enabled":       false,
		"agent_telemetry.enabled": false,
	})
	manager := packets.NewPoolManager[packets.Packet](packets.NewPool(cfg, 8192, nil))
	oob := packets.NewPoolManager[[]byte](ddsync.NewSlicePool[byte](32, 32))
	writer := NewTrafficCaptureWriter(depth, taggerfxmock.SetupFakeTagger(t))
	require.NoError(t, writer.RegisterSharedPoolManager(manager))
	require.NoError(t, writer.RegisterOOBPoolManager(oob))
	return writer, manager, oob
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

func TestWriterRepeatedCapture(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprintf("compressed=%v", compressed), func(t *testing.T) {
			writer, manager, oob := newTestCaptureWriter(t, 0)
			synctest.Test(t, func(t *testing.T) {
				var held *replay.CaptureBuffer
				for capture := range 2 {
					out := &testCaptureOutput{}
					session, err := writer.startCapture(out, time.Minute, compressed)
					require.NoError(t, err)
					var producers sync.WaitGroup
					for producer := range 2 {
						msg := captureTestBuffer(manager, oob, []byte(fmt.Sprintf("capture:%d/producer:%d|c", capture, producer)))
						if capture == 0 && producer == 0 {
							msg.Pid = 42
							msg.ContainerID = "container_id://first-capture"
							held = msg // Normal processing keeps ownership through the next capture.
						} else {
							require.NotSame(t, held.Buff, msg.Buff)
							require.NotSame(t, held.Oob, msg.Oob)
						}
						producers.Add(1)
						go func() {
							defer producers.Done()
							assert.True(t, writer.Enqueue(msg))
							if msg != held {
								manager.Put(msg.Buff)
								oob.Put(msg.Oob)
							}
						}()
					}
					producers.Wait()
					writer.StopAndWait(5 * time.Second)
					select {
					case <-session.done:
					default:
						t.Fatal("StopAndWait returned before capture finished")
					}
					require.False(t, writer.IsOngoing())
					require.True(t, out.closed)
					require.Equal(t, 1, manager.Count(), "normal processing still owns the first packet")
					require.Equal(t, 1, oob.Count())
					data := out.Bytes()
					if compressed {
						data, err = zstd.Decompress(nil, data)
						require.NoError(t, err)
					}
					reader := &TrafficCaptureReader{Contents: data, Version: int(datadogFileVersion)}
					reader.Seek(0)
					var payloads []string
					for range 2 {
						recorded, err := reader.ReadNext()
						require.NoError(t, err)
						payloads = append(payloads, string(recorded.Payload))
						require.Equal(t, []byte("credentials"), recorded.Ancillary)
					}
					require.ElementsMatch(t, []string{fmt.Sprintf("capture:%d/producer:0|c", capture), fmt.Sprintf("capture:%d/producer:1|c", capture)}, payloads)
					_, err = reader.ReadNext()
					require.ErrorIs(t, err, io.EOF)
					state, err := reader.ReadState()
					require.NoError(t, err)
					if capture == 0 {
						require.Equal(t, "container_id://first-capture", state.PidMap[42])
					} else {
						require.Empty(t, state.PidMap, "tagger state must not cross sessions")
					}
				}
				manager.Put(held.Buff)
				oob.Put(held.Oob)
				require.Zero(t, manager.Count())
				require.Zero(t, oob.Count())
				writer.StopAndWait(5 * time.Second) // Idempotent with no session.
			})
		})
	}
}

func TestWriterConcurrentStart(t *testing.T) {
	writer, _, _ := newTestCaptureWriter(t, 0)
	synctest.Test(t, func(t *testing.T) {
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
		<-first.done

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
		<-second.done
	})
}

func TestWriterStopUnblocksEnqueue(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("writeError=%v", fail), func(t *testing.T) {
			writer, manager, oob := newTestCaptureWriter(t, 1)
			synctest.Test(t, func(t *testing.T) {
				out := &blockedCaptureOutput{entered: make(chan struct{}), proceed: make(chan struct{}), fail: fail}
				var unblock sync.Once
				releaseWrite := func() { unblock.Do(func() { close(out.proceed) }) }
				defer releaseWrite()
				session, err := writer.startCapture(out, time.Minute, false)
				require.NoError(t, err)
				defer writer.StopCapture()
				first := captureTestBuffer(manager, oob, bytes.Repeat([]byte("x"), 8000))
				require.True(t, writer.Enqueue(first))
				manager.Put(first.Buff)
				oob.Put(first.Oob)
				<-out.entered // the consumer is now blocked in Write
				queued := captureTestBuffer(manager, oob, []byte("queued:1|c"))
				require.True(t, writer.Enqueue(queued))
				manager.Put(queued.Buff)
				oob.Put(queued.Oob)
				pending := captureTestBuffer(manager, oob, []byte("pending:1|c"))
				result := make(chan bool, 1)
				go func() {
					accepted := writer.Enqueue(pending)
					manager.Put(pending.Buff)
					oob.Put(pending.Oob)
					result <- accepted
				}()
				synctest.Wait() // The pending sender has retained its buffers and is blocked.
				require.Empty(t, result)
				require.Equal(t, 3, manager.Count())
				require.Equal(t, 3, oob.Count())
				if fail {
					releaseWrite() // consumer discovers the error and must wake senders itself
				} else {
					stopped := make(chan struct{})
					go func() { writer.StopCapture(); close(stopped) }()
					<-stopped
				}
				select {
				case accepted := <-result:
					require.False(t, accepted)
				case <-time.After(time.Second):
					t.Fatal("blocked enqueue did not wake on stop")
				}
				if !fail {
					require.Equal(t, 2, manager.Count(), "queued and writing buffers must remain retained")
					require.Equal(t, 2, oob.Count())
					require.True(t, writer.IsOngoing(), "must not admit another capture during drain")
					releaseWrite()
				}
				<-session.done
				require.True(t, out.closed)
				require.False(t, writer.IsOngoing())
				require.Zero(t, manager.Count())
				require.Zero(t, oob.Count())
				require.False(t, writer.Enqueue(pending))
				if !fail {
					reader := &TrafficCaptureReader{Contents: out.Bytes(), Version: int(datadogFileVersion)}
					reader.Seek(0)
					for _, payload := range [][]byte{bytes.Repeat([]byte("x"), 8000), []byte("queued:1|c")} {
						recorded, err := reader.ReadNext()
						require.NoError(t, err)
						require.Equal(t, payload, recorded.Payload)
					}
					_, err := reader.ReadNext()
					require.ErrorIs(t, err, io.EOF)
				}
			})
		})
	}
}

func TestWriterCaptureTimeout(t *testing.T) {
	writer, _, _ := newTestCaptureWriter(t, 0)
	synctest.Test(t, func(t *testing.T) {
		session, err := writer.startCapture(&testCaptureOutput{}, time.Second, false)
		require.NoError(t, err)
		synctest.Wait() // Register the timer before advancing virtual time.
		require.True(t, writer.IsOngoing())
		time.Sleep(time.Second)
		synctest.Wait()
		require.False(t, writer.IsOngoing())
		select {
		case <-session.done:
		default:
			t.Fatal("duration timer did not finish capture")
		}
	})
}

func TestWriterFlushError(t *testing.T) {
	writer, manager, oob := newTestCaptureWriter(t, 0)
	synctest.Test(t, func(t *testing.T) {
		out := &blockedCaptureOutput{entered: make(chan struct{}), proceed: make(chan struct{}), fail: true}
		close(out.proceed)
		session, err := writer.startCapture(out, time.Minute, false)
		require.NoError(t, err)
		msg := captureTestBuffer(manager, oob, []byte("buffered:1|c"))
		require.True(t, writer.Enqueue(msg))
		manager.Put(msg.Buff)
		oob.Put(msg.Oob)
		writer.StopCapture()
		<-session.done
		require.True(t, out.closed)
		require.Zero(t, manager.Count())
		require.Zero(t, oob.Count())
		next, err := writer.startCapture(&testCaptureOutput{}, time.Minute, false)
		require.NoError(t, err, "flush errors must not prevent subsequent captures")
		writer.StopCapture()
		<-next.done
	})
}
