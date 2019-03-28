package agent

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// serviceApp represents the app to which certain integration belongs to
const serviceApp = "app"

// ServiceMapper provides a cache layer over model.ServicesMetadata pipeline
// Used in conjunction with ServiceWriter: in-> ServiceMapper out-> ServiceWriter
type ServiceMapper struct {
	in    <-chan pb.ServicesMetadata
	out   chan<- pb.ServicesMetadata
	exit  chan bool
	done  sync.WaitGroup
	cache pb.ServicesMetadata
}

// NewServiceMapper returns an instance of ServiceMapper with the provided channels
func NewServiceMapper(in <-chan pb.ServicesMetadata, out chan<- pb.ServicesMetadata) *ServiceMapper {
	return &ServiceMapper{
		in:    in,
		out:   out,
		exit:  make(chan bool),
		cache: make(pb.ServicesMetadata),
	}
}

// Start runs the event loop in a non-blocking way
func (s *ServiceMapper) Start() {
	s.done.Add(1)

	go func() {
		defer watchdog.LogOnPanic()
		s.Run()
		s.done.Done()
	}()
}

// Stop gracefully terminates the event-loop
func (s *ServiceMapper) Stop() {
	close(s.exit)
	s.done.Wait()
}

// Run triggers the event-loop that consumes model.ServicesMeta
func (s *ServiceMapper) Run() {
	telemetryTicker := time.NewTicker(1 * time.Minute)
	defer telemetryTicker.Stop()

	for {
		select {
		case metadata := <-s.in:
			s.update(metadata)
		case <-telemetryTicker.C:
			log.Infof("total number of tracked services: %d", len(s.cache))
		case <-s.exit:
			return
		}
	}
}

func (s *ServiceMapper) update(metadata pb.ServicesMetadata) {
	var changes pb.ServicesMetadata

	for k, v := range metadata {
		if !s.shouldAdd(k, metadata) {
			continue
		}

		// We do this inside the for loop to avoid unnecessary memory allocations.
		// After few method executions, the cache will be warmed up and this section be skipped altogether.
		if changes == nil {
			changes = make(pb.ServicesMetadata)
		}

		changes[k] = v
	}

	if changes == nil {
		return
	}

	s.out <- changes

	for k, v := range changes {
		s.cache[k] = v
	}
}

func (s *ServiceMapper) shouldAdd(service string, metadata pb.ServicesMetadata) bool {
	cacheEntry, ok := s.cache[service]

	// No cache entry?
	if !ok {
		return true
	}

	// Cache entry came from service API?
	if _, ok = cacheEntry[serviceApp]; ok {
		return false
	}

	// New metadata value came from service API?
	_, ok = metadata[service][serviceApp]

	return ok
}

// appType is one of the pieces of information embedded in ServiceMetadata
const appType = "app_type"

// TraceServiceExtractor extracts service metadata from top-level spans
type TraceServiceExtractor struct {
	outServices chan<- pb.ServicesMetadata
}

// NewTraceServiceExtractor returns a new TraceServiceExtractor
func NewTraceServiceExtractor(out chan<- pb.ServicesMetadata) *TraceServiceExtractor {
	return &TraceServiceExtractor{out}
}

// Process extracts service metadata from top-level spans and sends it downstream
func (ts *TraceServiceExtractor) Process(t stats.WeightedTrace) {
	meta := make(pb.ServicesMetadata)

	for _, s := range t {
		if !s.TopLevel {
			continue
		}

		if _, ok := meta[s.Service]; ok {
			continue
		}

		if v := s.Type; len(v) > 0 {
			meta[s.Service] = map[string]string{appType: v}
		}
	}

	if len(meta) > 0 {
		ts.outServices <- meta
	}
}
