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
	"runtime"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

const (
	httpReadBufferCount = 100
)

type FullHttpTransaction struct {
	Txn             driver.HttpTransactionType
	RequestFragment []byte
}
type httpDriverInterface struct {
	driverHTTPHandle  *driver.Handle
	driverEventHandle windows.Handle

	readMux     sync.Mutex
	dataChannel chan []FullHttpTransaction
	eventLoopWG sync.WaitGroup
	closed      bool
}

func newDriverInterface(dh *driver.Handle) (*httpDriverInterface, error) {
	d := &httpDriverInterface{}
	err := d.setupHTTPHandle(dh)
	if err != nil {
		return nil, err
	}

	d.dataChannel = make(chan []FullHttpTransaction)
	return d, nil
}

func (di *httpDriverInterface) setupHTTPHandle(dh *driver.Handle) error {

	// enable HTTP on this handle
	settings := driver.HttpConfigurationSettings{
		MaxTransactions:        driver.HttpBatchSize * 2,
		NotificationThreshhold: driver.HttpBatchSize,
		MaxRequestFragment:     driver.HttpBufferSize,
	}

	err := windows.DeviceIoControl(dh.Handle,
		driver.EnableHttpIOCTL,
		(*byte)(unsafe.Pointer(&settings)),
		uint32(driver.HttpSettingsTypeSize),
		nil,
		uint32(0), nil, nil)
	if err != nil {
		log.Warnf("Failed to enable http in driver %v", err)
		return err
	}
	log.Infof("Enabled http in driver")

	di.driverHTTPHandle = dh

	u16eventname, err := windows.UTF16PtrFromString("Global\\DDNPMHttpTxnReadyEvent")
	di.driverEventHandle, err = windows.CreateEvent(nil, 1, 0, u16eventname)
	if err != nil {
		if err != windows.ERROR_ALREADY_EXISTS || di.driverEventHandle == windows.Handle(0) {
			log.Warnf("Failed to create driver event handle %v", err)
			return err
		}
		log.Infof("non-nil err, %v %v", di.driverEventHandle, err)
	}
	return nil
}

func (di *httpDriverInterface) readAllPendingTransactions() {
	di.readMux.Lock()
	defer di.readMux.Unlock()
	count := int(0)
	for {
		txns, err := di.readPendingTransactions()
		if err != nil {
			log.Warnf("Error reading http transaction buffer: %v", err)
			break
		}
		if txns == nil && err == nil {
			// no transactions to read
			break
		}
		count += len(txns)
		di.dataChannel <- txns
	}
	log.Infof("Read all pending transactions read %d transactions", count)
}

func (di *httpDriverInterface) startReadingBuffers() {
	di.eventLoopWG.Add(1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer di.eventLoopWG.Done()

		for {
			windows.WaitForSingleObject(di.driverEventHandle, windows.INFINITE)
			// dbtodo -- downgrade or remove this message
			log.Infof("Driver signalled batch is ready")
			if di.closed {
				break
			}
			di.readAllPendingTransactions()
		}
	}()
}

//func (di *httpDriverInterface) flushPendingTransactions() ([]driver.HttpTransactionType, error) {
func (di *httpDriverInterface) readPendingTransactions() ([]FullHttpTransaction, error) {
	var (
		bytesRead uint32
		buf       = make([]byte, (driver.HttpTransactionTypeSize+driver.HttpBufferSize)*driver.HttpBatchSize)
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
	if bytesRead == 0 {
		return nil, nil
	}
	transactionBatch := make([]FullHttpTransaction, 0)

	for i := uint32(0); i < bytesRead; {
		var tx FullHttpTransaction
		tx.Txn = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf[i]))
		tx.RequestFragment = make([]byte, tx.Txn.MaxRequestFragment)
		i += driver.HttpTransactionTypeSize
		copy(tx.RequestFragment, buf[i:i+uint32(tx.Txn.MaxRequestFragment)])
		i += uint32(tx.Txn.MaxRequestFragment)

		transactionBatch = append(transactionBatch, tx)
	}

	return transactionBatch, nil
}

func (di *httpDriverInterface) getStats() (map[string]int64, error) {
	return di.driverHTTPHandle.GetStatsForHandle()
}

func (di *httpDriverInterface) close() error {
	di.closed = true
	windows.SetEvent(di.driverEventHandle)
	di.eventLoopWG.Wait()
	windows.CloseHandle(di.driverEventHandle)
	close(di.dataChannel)

	return nil
}
