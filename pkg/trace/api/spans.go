// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package api

import (
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/cache"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/tinylib/msgp/msgp"
)

const maxCacheSize = 200 * 1024 * 1024 // 200MB

// spanFunnel is an http.Server which receives incoming spans and funnels them into complete traces.
// Use newSpanFunnel to initialize.
type spanFunnel struct {
	// receiver specifies the receiver that is using this spanFunnel.
	receiver *HTTPReceiver
	// reassembler caches spans and evicts complete traces.
	reassembler *cache.Reassembler
	// wg waits for all workers to exit.
	wg sync.WaitGroup
}

func (r *HTTPReceiver) newSpanFunnel() *spanFunnel {
	out := make(chan *cache.EvictedTrace, 5000)
	e := spanFunnel{
		receiver:    r,
		reassembler: cache.NewReassembler(out, maxCacheSize),
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			for et := range out {
				r.processTraces(et.Source, "", pb.Traces{et.Spans})
			}
		}()
	}
	return &e
}

// Stop stops the span funnel.
func (sf *spanFunnel) Stop() {
	sf.reassembler.Stop()
	sf.wg.Wait()
}

// ServeHTTP implements http.Handler.
func (sf *spanFunnel) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ts := sf.receiver.tagStats(req)
	req.Body = NewLimitedReader(req.Body, maxRequestBodyLength)
	defer req.Body.Close()

	var spans pb.Trace
	if err := msgp.Decode(req.Body, &spans); err != nil {
		log.Errorf("Could not decode payload: %v", err)
		httpDecodingError(err, []string{
			"handler:spans",
			fmt.Sprintf("lang:%s", ts.Lang),
			fmt.Sprintf("tracer_version:%s", ts.TracerVersion),
		}, w)
		return
	}

	atomic.AddInt64(&ts.SpansReceived, int64(len(spans)))
	atomic.AddInt64(&ts.TracesBytes, int64(req.Body.(*LimitedReader).Count))
	atomic.AddInt64(&ts.PayloadAccepted, 1)

	httpRateByService(w, sf.receiver.dynConf)

	// if (*Reassembler).Add ends up blocking, make sure it doesn't end up blocking the request
	go func() {
		defer timing.Since("datadog.trace_agent.reassembler.add_time_ms", time.Now())
		sf.reassembler.Add(&cache.Item{
			Source: ts,
			Spans:  spans,
		})
	}()
}
