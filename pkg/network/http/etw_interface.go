// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/etw"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpEtwInterface struct {
	dataChannel chan []driver.HttpTransactionType
	eventLoopWG sync.WaitGroup
}

func newHttpEtwInterface() *httpEtwInterface {
	hei := &httpEtwInterface{}
	hei.dataChannel = make(chan []driver.HttpTransactionType)
	return hei
}

func (hei *httpEtwInterface) setMaxFlows(maxFlows uint64) error {
	log.Debugf("Setting max flows in driver http filter to %v", maxFlows)
	return nil
}

func (hei *httpEtwInterface) startReadingHttpTransaction() {
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
			// transactionSize := uint32(driver.HttpTransactionTypeSize)
			// batchSize := bytesRead / transactionSize
			// transactionBatch := make([]driver.HttpTransactionType, batchSize)

			// for i := uint32(0); i < batchSize; i++ {
			// 	transactionBatch[i] = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf.Data[i*transactionSize]))
			// }

			// hei.dataChannel <- transactionBatch
		}
	}()
}

func (hei *httpEtwInterface) flushPendingTransactions() ([]driver.HttpTransactionType, error) {
	// var (
	// 	bytesRead uint32
	// 	buf       = make([]byte, driver.HttpTransactionTypeSize*driver.HttpBatchSize)
	// )

	// transactionSize := uint32(driver.HttpTransactionTypeSize)
	// batchSize := bytesRead / transactionSize
	// transactionBatch := make([]driver.HttpTransactionType, batchSize)

	// for i := uint32(0); i < batchSize; i++ {
	// 	transactionBatch[i] = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf[i*transactionSize]))
	// }

	// return transactionBatch, nil
	return nil, nil
}

func (hei *httpEtwInterface) getStats() (map[string]int64, error) {
	return nil, nil
}

func (hei *httpEtwInterface) close() error {
	etw.StopEtw("ddnpm-httpservice")

	hei.eventLoopWG.Wait()
	close(hei.dataChannel)

	return nil
}
