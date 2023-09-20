// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

type EtwInterface struct {
	maxEntriesBuffered int
	DataChannel        chan []WinHttpTransaction
	eventLoopWG        sync.WaitGroup
	captureHTTP        bool
	captureHTTPS       bool
}

func NewEtwInterface(c *config.Config) *EtwInterface {
	return &EtwInterface{
		maxEntriesBuffered: c.MaxHTTPStatsBuffered,
		DataChannel:        make(chan []WinHttpTransaction),
		captureHTTPS:       c.EnableNativeTLSMonitoring,
		captureHTTP:        c.EnableHTTPMonitoring,
	}
}

func (hei *EtwInterface) SetCapturedProtocols(http, https bool) {
	hei.captureHTTP = http
	hei.captureHTTPS = https
	SetEnabledProtocols(http, https)
}
func (hei *EtwInterface) SetMaxFlows(maxFlows uint64) {
	log.Debugf("Setting max flows in ETW http source to %v", maxFlows)
	SetMaxFlows(maxFlows)
}

func (hei *EtwInterface) SetMaxRequestBytes(maxRequestBytes uint64) {
	log.Debugf("Setting max request bytes in ETW http source to to %v", maxRequestBytes)
	SetMaxRequestBytes(maxRequestBytes)
}

func (hei *EtwInterface) StartReadingHttpFlows() {
	hei.eventLoopWG.Add(2)

	startingEtwChan := make(chan struct{})

	// Currently ETW needs be started on a separate thread
	// because it is blocked until subscription is stopped
	go func() {
		defer hei.eventLoopWG.Done()

		// By default this function call never exits and its callbacks or rather events
		// will be returned on the very the same thread until ETW is canceled via
		// etw.StopEtw(). There is asynchronous flag which implicitly will create a real
		// (Windows API) thread but it is not tested yet.
		log.Infof("Starting ETW HttpService subscription")

		startingEtwChan <- struct{}{}

		err := etw.StartEtw("ddnpm-httpservice", etw.EtwProviderHttpService, hei)

		if err == nil {
			log.Infof("ETW HttpService subscription completed")
		} else {
			log.Errorf("ETW HttpService subscription failed with error %v", err)
		}
	}()

	log.Infof("BEFORE hei.eventLoopWG.Done")

	// Start reading accumulated HTTP transactions
	go func() {
		defer hei.eventLoopWG.Done()
		defer close(startingEtwChan)

		// Block until we get go ahead signal
		<-startingEtwChan

		// We need to make sure that we are invoked when etw.StartEtw() is called but
		// we cannot wait until it exits because it exits only when Agent exit. There
		// shoulbe be more elegant ways to deal with that, perhaps adding dedicated
		// callback from CGO but for now let's sleep for a second
		time.Sleep(time.Second)

		log.Infof("Starting etw.ReadHttpTx()")

		for {
			// etw.ReadHttpTx() should be executed after another thread above executes etw.StartEtw()
			// Probably additional synchronization is required
			httpTxs, err := ReadHttpTx()
			if err != nil {
				log.Infof("ETW HttpService subscriptions is stopped. Stopping http monitoring")
				return
			}

			if len(httpTxs) > 0 {
				hei.DataChannel <- httpTxs
			}

			// need a better signalling mechanism
			time.Sleep(3 * time.Second)
		}
	}()
}

func (hei *EtwInterface) Close() {
	etw.StopEtw("ddnpm-httpservice")

	hei.eventLoopWG.Wait()
	close(hei.DataChannel)
}
