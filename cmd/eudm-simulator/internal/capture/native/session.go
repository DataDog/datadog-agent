// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows || darwin

// Package native runs capture collectors only on their real platform.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sort"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type sample struct {
	stream schema.Stream
	offset time.Duration
	data   json.RawMessage
}
type session struct {
	sanitizer         *capture.Sanitizer
	start             time.Time
	currentOffset     time.Duration
	mu                sync.Mutex
	pending           []sample
	profile           schema.Profile
	counts            map[schema.Stream]int
	offsets           map[schema.Stream][]time.Duration
	scheduledCadences map[schema.Stream]time.Duration
}

func (s *session) add(stream schema.Stream, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cannot encode sanitized %s sample", stream)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, sample{stream: stream, offset: s.currentOffset, data: data})
	return nil
}

func (s *session) Series(source metrics.SerieSource) (metrics.SerieSource, error) {
	clean, err := s.sanitizer.Series(source)
	if err != nil {
		return nil, err
	}
	owned := clean.(*capture.SeriesSource)
	if len(owned.Series) == 0 {
		return clean, nil
	}
	for _, serie := range owned.Series {
		if !slices.Contains(s.profile.MetricNames, serie.Name) {
			s.profile.MetricNames = append(s.profile.MetricNames, serie.Name)
		}
		for i := range serie.Points {
			serie.Points[i].Ts -= float64(s.start.UnixNano()) / 1e9
		}
	}
	portable, err := telemetry.NewMetricSample(owned.Series)
	if err != nil {
		return nil, err
	}
	return clean, s.add(schema.Metrics, portable)
}

func (s *session) HostMetadata(raw marshaler.JSONMarshaler) (marshaler.JSONMarshaler, error) {
	clean, err := s.sanitizer.HostMetadata(raw)
	if err != nil {
		return nil, err
	}
	return clean, s.add(schema.HostMetadata, clean)
}

func (s *session) Process(_ string, messages []model.MessageBody) ([]model.MessageBody, error) {
	result := make([]model.MessageBody, 0, len(messages))
	for _, raw := range messages {
		switch msg := raw.(type) {
		case *model.CollectorProc:
			clean := s.sanitizer.Process(msg)
			if clean.Info != nil && clean.Info.TotalMemory > 0 {
				s.profile.MemoryBytes = uint64(clean.Info.TotalMemory)
			}
			for _, proc := range clean.Processes {
				proc.CreateTime -= s.start.UnixMilli()
				if proc.Command != nil && !slices.Contains(s.profile.ProcessNames, proc.Command.Comm) {
					s.profile.ProcessNames = append(s.profile.ProcessNames, proc.Command.Comm)
				}
			}
			if err := s.add(schema.Processes, clean); err != nil {
				return nil, err
			}
			result = append(result, clean)
		case *model.CollectorConnections:
			clean := s.sanitizer.Connections(msg)
			for _, conn := range clean.Connections {
				selector := telemetry.ConnectionSelector(conn)
				if !slices.Contains(s.profile.ConnectionSelectors, selector) {
					s.profile.ConnectionSelectors = append(s.profile.ConnectionSelectors, selector)
				}
			}
			if err := s.add(schema.Connections, clean); err != nil {
				return nil, err
			}
			result = append(result, clean)
		default:
			return nil, errors.New("capture received an unsupported process message")
		}
	}
	return result, nil
}

type scheduledCollector struct {
	name     string
	next     time.Time
	interval time.Duration
	run      func(context.Context) (time.Duration, error)
}

var errNativeCollection = errors.New("native collection unavailable")

// Run collects on native schedules until complete coverage or context deadline.
// No network-capable transport exists anywhere in the intake pipeline.
func Run(ctx context.Context, directory string) error {
	if len(version.FullCommit) != 40 {
		return errors.New("capture requires a revision-stamped build; use dda inv eudm-simulator.build")
	}
	// Capture reports coverage through fixed, local errors. Suppress native
	// diagnostic logs, whose OS error strings can contain real user paths.
	log.SetupLogger(log.Disabled(), "error")
	sanitizer, err := capture.NewSanitizer()
	if err != nil {
		return err
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	s := &session{sanitizer: sanitizer, start: time.Now(), profile: schema.Profile{OS: platform, Architecture: runtime.GOARCH}, counts: map[schema.Stream]int{}, offsets: map[schema.Stream][]time.Duration{}, scheduledCadences: map[schema.Stream]time.Duration{}}
	writer, err := bundle.NewWriter(directory, bundle.Manifest{AgentVersion: version.AgentVersion, AgentCommit: version.FullCommit})
	if err != nil {
		return err
	}
	recorder := output.NewRecorder()
	destinations, err := (safety.Config{Site: safety.Site}).Resolve(func(string) string { return "" })
	if err != nil {
		return err
	}
	pipeline, err := output.New(ctx, destinations, "recording-only", recorder, output.Options{Serializer: s, Process: s.Process})
	if err != nil {
		return err
	}
	defer pipeline.Close()
	collectors, cleanup, err := newCollectors(ctx, s, pipeline)
	if err != nil {
		return err
	}
	defer cleanup()
	required := []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software}
	if platform == "windows" {
		required = append(required, schema.Connections)
	}
	for i := range collectors {
		collectors[i].next = s.start
	}
	previous := 0
	for {
		missing := []string{}
		for _, stream := range required {
			minimum := 1
			if stream == schema.Metrics || stream == schema.Processes || stream == schema.Connections {
				minimum = 2
			}
			if s.counts[stream] < minimum {
				missing = append(missing, string(stream))
			}
		}
		if len(missing) == 0 {
			s.profile.Streams = required
			cadences := map[schema.Stream]time.Duration{}
			for _, stream := range required {
				offsets := s.offsets[stream]
				if len(offsets) > 1 {
					cadences[stream] = (offsets[len(offsets)-1] - offsets[0]) / time.Duration(len(offsets)-1)
				} else {
					cadences[stream] = s.scheduledCadences[stream]
				}
			}
			sort.Strings(s.profile.MetricNames)
			sort.Strings(s.profile.ProcessNames)
			sort.Strings(s.profile.SoftwareNames)
			sort.Strings(s.profile.ConnectionSelectors)
			_, err := writer.Complete(time.Since(s.start), s.profile, cadences)
			return err
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("capture incomplete; missing streams %v: %w", missing, err)
		}
		next := 0
		for i := range collectors {
			if collectors[i].next.Before(collectors[next].next) {
				next = i
			}
		}
		c := &collectors[next]
		wait := time.Until(c.next)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				continue
			case <-timer.C:
			}
		}
		s.currentOffset = time.Since(s.start)
		interval, collectionErr := c.run(ctx)
		if interval <= 0 {
			interval = c.interval
		}
		c.interval = interval
		c.next = time.Now().Add(interval)
		// Native collection failures are retried on that collector's schedule.
		// Do not expose raw OS error strings, which can include paths or users.
		if collectionErr != nil {
			if errors.Is(collectionErr, errNativeCollection) {
				continue
			}
			return collectionErr
		}
		if err := pipeline.Wait(ctx); err != nil {
			return err
		}
		s.mu.Lock()
		pending := s.pending
		s.pending = nil
		s.mu.Unlock()
		if len(pending) == 0 {
			continue
		}
		refs, err := recorder.Wait(ctx, previous+1)
		if err != nil {
			return err
		}
		// The native scheduler serializes capture submissions. Every callback in
		// this collection shares its fully drained batch of wire references.
		for _, sample := range pending {
			s.scheduledCadences[sample.stream] = interval
			if err := writer.Append(sample.stream, sample.offset, sample.data, refs[previous:]); err != nil {
				return err
			}
			offsets := s.offsets[sample.stream]
			if len(offsets) == 0 || offsets[len(offsets)-1] != sample.offset {
				s.counts[sample.stream]++
				s.offsets[sample.stream] = append(offsets, sample.offset)
			}
		}
		previous = len(refs)
	}
}
