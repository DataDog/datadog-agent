// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpEtwInterface struct {
	dataChannel chan []etw.ConnHttp
	eventLoopWG sync.WaitGroup
}

func newHttpEtwInterface() *httpEtwInterface {
	hei := &httpEtwInterface{}
	hei.dataChannel = make(chan []etw.ConnHttp)
	return hei
}

func (hei *httpEtwInterface) setMaxFlows(maxFlows uint64) {
	log.Debugf("Setting max flows in driver http filter to %v", maxFlows)
	etw.SetMaxFlows(maxFlows)
}

func (hei *httpEtwInterface) startReadingHttpFlows() {
	hei.eventLoopWG.Add(2)

	// Currently ETW needs be started on a separate thread
	// becauise it is blocked until subscription is stopped
	go func() {
		defer hei.eventLoopWG.Done()

		etw.StartEtw("ddnpm-httpservice")
	}()

	// Start reading accumulated HTTP transactions
	go func() {
		defer hei.eventLoopWG.Done()

		for {
			httpConns := etw.ReadConnHttp()
			if len(httpConns) > 0 {
				hei.dataChannel <- httpConns
			}

			// need a better signalling mechanism
			time.Sleep(5 * time.Second)
		}
	}()
}

func (hei *httpEtwInterface) getStats() (map[string]int64, error) {
	return nil, nil
}

func (hei *httpEtwInterface) close() {
	etw.StopEtw("ddnpm-httpservice")

	hei.eventLoopWG.Wait()
	close(hei.dataChannel)
}
