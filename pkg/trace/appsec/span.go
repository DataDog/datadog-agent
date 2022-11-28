// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	cryptorand "crypto/rand"
	"math"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

type span struct {
	*pb.Span
}

func startSpan(traceID, parentID uint64, name, typ string) span {
	start := time.Now().UnixNano()
	spanID := generateSpanID(start)
	if traceID == 0 {
		traceID = spanID
	}
	return span{
		Span: &pb.Span{
			TraceID:  traceID,
			ParentID: parentID,
			SpanID:   spanID,
			Start:    start,
			Name:     name,
			Type:     typ,
			Meta:     map[string]string{},
			Metrics:  map[string]float64{},
		},
	}
}

func (s *span) finish() {
	s.Duration = time.Now().UnixNano() - s.Start
}

type httpSpan struct {
	span
}

func startHTTPRequestSpan(traceID, parentID uint64, resource string) httpSpan {
	sp := startSpan(traceID, parentID, "http.request", "web")
	sp.Resource = resource
	return httpSpan{sp}
}

func (s *httpSpan) SetMethod(m string)     { s.Meta["http.method"] = m }
func (s *httpSpan) SetURL(u string)        { s.Meta["http.url"] = u }
func (s *httpSpan) SetUserAgent(ua string) { s.Meta["http.useragent"] = ua }
func (s *httpSpan) SetRequestHeaders(headers map[string]string) {
	for k, v := range headers {
		s.Meta["http.request.headers."+k] = v
	}
}

// generateSpanID returns a random uint64 that has been XORd with the startTime.
// This is done to get around the 32-bit random seed limitation that may create collisions if there is a large number
// of services all generating spans.
func generateSpanID(startTime int64) uint64 {
	return random.Uint64() ^ uint64(startTime)
}

// random holds a thread-safe source of random numbers.
var random *rand.Rand

func init() {
	var seed int64
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(math.MaxInt64))
	if err == nil {
		seed = n.Int64()
	} else {
		log.Warn("cannot generate random seed: %v; using current time", err)
		seed = time.Now().UnixNano()
	}
	random = rand.New(&safeSource{
		source: rand.NewSource(seed),
	})
}

// safeSource holds a thread-safe implementation of rand.Source64.
type safeSource struct {
	source rand.Source
	sync.Mutex
}

func (rs *safeSource) Int63() int64 {
	rs.Lock()
	n := rs.source.Int63()
	rs.Unlock()

	return n
}

func (rs *safeSource) Uint64() uint64 { return uint64(rs.Int63()) }

func (rs *safeSource) Seed(seed int64) {
	rs.Lock()
	rs.source.Seed(seed)
	rs.Unlock()
}
