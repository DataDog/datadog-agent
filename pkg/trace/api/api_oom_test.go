// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package api

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestOOMKill(t *testing.T) {
	kills := atomic.NewUint64(0)

	defer func(old func(string, ...interface{})) { killProcess = old }(killProcess)
	killProcess = func(format string, a ...interface{}) {
		if format != "OOM" {
			t.Fatalf("wrong message: %s", fmt.Sprintf(format, a...))
		}
		kills.Inc()
	}

	conf := config.New()
	conf.Endpoints[0].APIKey = "apikey_2"
	conf.WatchdogInterval = time.Millisecond
	conf.MaxMemory = 0.5 * 1000 * 1000 // 0.5M

	r := newTestReceiverFromConfig(conf)
	r.Start()
	defer r.Stop()
	go func() {
		for range r.out {
		}
	}()

	var traces pb.Traces
	for i := 0; i < 20; i++ {
		traces = append(traces, testutil.RandomTrace(10, 20))
	}
	data := msgpTraces(t, traces)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for tries := 0; tries < 50; tries++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post("http://localhost:8126/v0.4/traces", "application/msgpack", bytes.NewReader(data))
			if err != nil {
				errs <- err
				return
			}
			resp.Body.Close()
		}()
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-timeout:
			break loop
		default:
			if kills.Load() > 1 {
				return
			}
			time.Sleep(conf.WatchdogInterval)
		}
	}
	t.Fatal("didn't get OOM killed")
}
