// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

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

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

///const (
///	httpReadBufferCount = 100
///)

//nolint:revive // TODO(WKIT) Fix revive linter
type WinHttpTransaction struct {
	Txn             driver.HttpTransactionType
	RequestFragment []byte

	// ... plus some extra that's only valid when it's an ETW transactoin
	AppPool string

	// <<<MORE ETW HttpService DETAILS>>>
	// We can track FULL url and few other attributes. However it will require much memory.
	// Search for <<<MORE ETW HttpService DETAILS>>> top find all places to be uncommented
	// if such tracking is desired
	//
	// Url           string
	SiteID   uint32
	SiteName string
	// HeaderLength  uint32
	// ContentLength uint32
}

//nolint:revive // TODO(WKIT) Fix revive linter
type HttpDriverInterface struct {
	driverHTTPHandle  driver.Handle
	driverEventHandle windows.Handle

	readMux     sync.Mutex
	DataChannel chan []WinHttpTransaction
	eventLoopWG sync.WaitGroup
	closed      bool
	// configuration entries
	maxTransactions       uint64
	notificationThreshold uint64
	maxRequestFragment    uint64
}

//nolint:revive // TODO(WKIT) Fix revive linter
func NewDriverInterface(c *config.Config, dh driver.Handle) (*HttpDriverInterface, error) {
	d := &HttpDriverInterface{
		maxTransactions:       uint64(c.MaxTrackedHTTPConnections),
		notificationThreshold: uint64(c.HTTPNotificationThreshold),
		maxRequestFragment:    uint64(c.HTTPMaxRequestFragment),
	}
	err := d.setupHTTPHandle(dh)
	if err != nil {
		return nil, err
	}

	d.DataChannel = make(chan []WinHttpTransaction)
	return d, nil
}

func (di *HttpDriverInterface) setupHTTPHandle(dh driver.Handle) error {

	di.driverHTTPHandle = dh
	// enable HTTP on this handle
	settings := driver.HttpConfigurationSettings{
		MaxTransactions:        di.maxTransactions,
		NotificationThreshold:  di.notificationThreshold,
		MaxRequestFragment:     uint16(di.maxRequestFragment),
		EnableAutoETWExclusion: uint16(1),
	}

	err := dh.DeviceIoControl(
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

	u16eventname, _ := windows.UTF16PtrFromString("Global\\DDNPMHttpTxnReadyEvent")
	di.driverEventHandle, err = windows.CreateEvent(nil, 1, 0, u16eventname)
	if err != nil {
		if err != windows.ERROR_ALREADY_EXISTS || di.driverEventHandle == windows.Handle(0) {
			log.Warnf("Failed to create driver event handle %v", err)
			return err
		}
	}
	return nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (di *HttpDriverInterface) ReadAllPendingTransactions() {
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
		di.DataChannel <- txns
	}
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (di *HttpDriverInterface) StartReadingBuffers() {
	di.eventLoopWG.Add(1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer di.eventLoopWG.Done()

		for {
			_, _ = windows.WaitForSingleObject(di.driverEventHandle, windows.INFINITE)
			if di.closed {
				break
			}
			di.ReadAllPendingTransactions()
		}
	}()
}

// func (di *httpDriverInterface) flushPendingTransactions() ([]driver.HttpTransactionType, error) {
func (di *HttpDriverInterface) readPendingTransactions() ([]WinHttpTransaction, error) {
	var (
		bytesRead uint32
		buf       = make([]byte, (driver.HttpTransactionTypeSize+di.maxRequestFragment)*di.maxTransactions)
	)

	err := di.driverHTTPHandle.DeviceIoControl(
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
	transactionBatch := make([]WinHttpTransaction, 0)

	for i := uint32(0); i < bytesRead; {
		var tx WinHttpTransaction
		tx.Txn = *(*driver.HttpTransactionType)(unsafe.Pointer(&buf[i]))
		tx.RequestFragment = make([]byte, tx.Txn.MaxRequestFragment)
		i += driver.HttpTransactionTypeSize
		copy(tx.RequestFragment, buf[i:i+uint32(tx.Txn.MaxRequestFragment)])
		i += uint32(tx.Txn.MaxRequestFragment)
		transactionBatch = append(transactionBatch, tx)
	}

	return transactionBatch, nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (di *HttpDriverInterface) Close() error {
	di.closed = true
	windows.SetEvent(di.driverEventHandle)
	di.eventLoopWG.Wait()
	windows.CloseHandle(di.driverEventHandle)
	close(di.DataChannel)

	return nil
}
