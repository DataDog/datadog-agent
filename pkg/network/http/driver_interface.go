// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

/*
#include <stdlib.h>
#include <memory.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

const (
	httpReadBufferCount = 100
)

type httpDriverInterface struct {
	driverHTTPHandle *driver.Handle
	readBuffers      []*driver.ReadBuffer
	iocp             windows.Handle

	dataChannel chan []driver.HttpTransactionType
	eventLoopWG sync.WaitGroup
}

func newDriverInterface(dh *driver.Handle) (*httpDriverInterface, error) {
	d := &httpDriverInterface{}
	err := d.setupHTTPHandle(dh)
	if err != nil {
		return nil, err
	}

	d.dataChannel = make(chan []driver.HttpTransactionType)
	return d, nil
}

func (di *httpDriverInterface) setupHTTPHandle(dh *driver.Handle) error {

	// enable HTTP on this handle
	enabled := uint8(1)

	err := windows.DeviceIoControl(dh.Handle,
		driver.EnableHttpIOCTL,
		(*byte)(unsafe.Pointer(&enabled)),
		uint32(unsafe.Sizeof(enabled)),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to enable http in driver %v", err)
		return err
	}
	log.Infof("Enabled http in driver")

	iocp, buffers, err := driver.PrepareCompletionBuffers(dh.Handle, httpReadBufferCount)
	if err != nil {
		return err
	}

	di.driverHTTPHandle = dh
	di.iocp = iocp
	di.readBuffers = buffers
	return nil
}

func (di *httpDriverInterface) startReadingBuffers() {
	di.eventLoopWG.Add(1)
	go func() {
		defer di.eventLoopWG.Done()

		for {
			buf, bytesRead, err := driver.GetReadBufferWhenReady(di.iocp)
			if iocpIsClosedError(err) {
				log.Debug("http io completion port is closed. stopping http monitoring")
				return
			}
			if err != nil {
				log.Warnf("Error reading http transaction buffer: %v", err)
				continue
			}

			transactionSize := uint32(driver.HttpTransactionTypeSize)
			batchSize := bytesRead / transactionSize
			transactionBatch := make([]driver.HttpTransactionType, batchSize)

			for i := uint32(0); i < batchSize; i++ {
				transactionBatch[i] = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf.Data[i*transactionSize]))
			}

			di.dataChannel <- transactionBatch

			err = driver.StartNextRead(di.driverHTTPHandle.Handle, buf)
			if err != nil && err != windows.ERROR_IO_PENDING {
				log.Warnf("Error starting next http transaction read: %v")
			}
		}
	}()
}

func iocpIsClosedError(err error) bool {
	if err == nil {
		return false
	}
	// ERROR_OPERATION_ABORTED or ERROR_ABANDONED_WAIT_0 indicates that the iocp handle was closed
	// during a call to GetQueuedCompletionStatus.
	// ERROR_INVALID_HANDLE indicates that the handle was closed prior to the call being made.
	return errors.Is(err, windows.ERROR_OPERATION_ABORTED) ||
		errors.Is(err, windows.ERROR_ABANDONED_WAIT_0) ||
		errors.Is(err, windows.ERROR_INVALID_HANDLE)
}

func (di *httpDriverInterface) flushPendingTransactions() ([]driver.HttpTransactionType, error) {
	var (
		bytesRead uint32
		buf       = make([]byte, driver.HttpTransactionTypeSize*driver.HttpBatchSize)
	)

	err := windows.DeviceIoControl(di.driverHTTPHandle.Handle,
		driver.FlushPendingHttpTxnsIOCTL,
		&driver.DdAPIVersionBuf[0], uint32(len(driver.DdAPIVersionBuf)),
		&buf[0], uint32(len(buf)),
		&bytesRead,
		nil)

	if err != nil {
		log.Infof("http flushPendingTransactions error %v", err)
		return nil, err
	}
	log.Infof("http flushPendingTransactin bytes %v", bytesRead)

	transactionSize := uint32(driver.HttpTransactionTypeSize)
	batchSize := bytesRead / transactionSize
	transactionBatch := make([]driver.HttpTransactionType, batchSize)

	for i := uint32(0); i < batchSize; i++ {
		transactionBatch[i] = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf[i*transactionSize]))
	}

	return transactionBatch, nil
}

func (di *httpDriverInterface) getStats() (map[string]int64, error) {
	return di.driverHTTPHandle.GetStatsForHandle()
}

func (di *httpDriverInterface) close() error {
	err := di.closeDriverHandles()
	di.eventLoopWG.Wait()
	close(di.dataChannel)

	for _, buf := range di.readBuffers {
		C.free(unsafe.Pointer(buf))
	}
	di.readBuffers = nil
	return err
}

func (di *httpDriverInterface) closeDriverHandles() error {
	err := windows.CancelIoEx(di.driverHTTPHandle.Handle, nil)
	if err != nil && err != windows.ERROR_NOT_FOUND {
		return fmt.Errorf("error cancelling outstanding HTTP io requests: %w", err)
	}
	err = windows.CloseHandle(di.iocp)
	if err != nil {
		return fmt.Errorf("error closing HTTP io completion handle: %w", err)
	}
	err = di.driverHTTPHandle.Close()
	if err != nil {
		return fmt.Errorf("error closing driver HTTP file handle: %w", err)
	}
	return nil
}
