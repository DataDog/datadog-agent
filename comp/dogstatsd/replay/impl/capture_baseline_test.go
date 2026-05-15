// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package replayimpl

import (
	"testing"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

func BenchmarkMilestone0CaptureEnqueue(b *testing.B) {
	b.Run("capture_off", func(b *testing.B) {
		writer := NewTrafficCaptureWriter(1, taggerfxmock.SetupFakeTagger(b))
		msg := &replay.CaptureBuffer{}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = writer.Enqueue(msg)
		}
	})

	b.Run("capture_on", func(b *testing.B) {
		writer := NewTrafficCaptureWriter(1024, taggerfxmock.SetupFakeTagger(b))
		writer.Lock()
		writer.accepting = true
		writer.Unlock()

		done := make(chan struct{})
		go func() {
			for i := 0; i < b.N; i++ {
				<-writer.Traffic
			}
			close(done)
		}()
		msg := &replay.CaptureBuffer{}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = writer.Enqueue(msg)
		}
		b.StopTimer()
		<-done
	})
}
