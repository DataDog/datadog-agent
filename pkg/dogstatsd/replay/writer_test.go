// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func TestWriter(t *testing.T) {
	atomic.StoreInt64(&inMemoryFs, 1)
	defer atomic.StoreInt64(&inMemoryFs, 0)

	writer := NewTrafficCaptureWriter("foo/bar", 1)

	// register pools
	manager := packets.NewPoolManager(packets.NewPool(config.Datadog.GetInt("dogstatsd_buffer_size")))
	oobManager := packets.NewPoolManager(packets.NewPool(32))

	writer.RegisterSharedPoolManager(manager)
	writer.RegisterOOBPoolManager(oobManager)

	var wg sync.WaitGroup
	const iterations = 5

	start := make(chan struct{})

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		close(start)
		writer.Capture(5 * time.Second)
	}(&wg)

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		<-start

		for i := 0; i < iterations; i++ {
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

			writer.Enqueue(buff)
			time.Sleep(500 * time.Millisecond)
		}

		writer.StopCapture()
	}(&wg)

	wg.Wait()

	// assert file
	writer.RLock()
	assert.NotNil(t, writer.testFile)
	assert.False(t, writer.ongoing)

	stats, _ := writer.testFile.Stat()
	assert.Greater(t, stats.Size(), int64(0))

	fp := writer.testFile
	writer.RUnlock()

	fp.Seek(0, io.SeekStart)

	buf := bytes.NewBuffer(nil)
	_, _ = io.Copy(buf, fp)
	reader := &TrafficCaptureReader{
		Contents: buf.Bytes(),
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
	assert.Equal(t, cnt, iterations)
}
