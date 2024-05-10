// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
)

//nolint:revive // TODO(WKIT) Fix revive linter
type EtwInterface struct {
	maxEntriesBuffered int
	DataChannel        chan []WinHttpTransaction
	eventLoopWG        sync.WaitGroup
	captureHTTP        bool
	captureHTTPS       bool
	requestSize        int64

	// ETW component
	httpguid windows.GUID
	session  etw.Session
}

// NewEtwInterface returns a new EtwInterface instance
func NewEtwInterface(c *config.Config) (*EtwInterface, error) {
	ei := &EtwInterface{
		maxEntriesBuffered: c.MaxHTTPStatsBuffered,
		DataChannel:        make(chan []WinHttpTransaction),
		captureHTTPS:       c.EnableNativeTLSMonitoring,
		captureHTTP:        c.EnableHTTPMonitoring,
		requestSize:        c.HTTPMaxRequestFragment,
	}
	etwSessionName := "SystemProbeUSM_ETW"
	etwcomp, err := etwimpl.NewEtw()
	if err != nil {
		return nil, err
	}

	ei.session, err = etwcomp.NewSession(etwSessionName, func(cfg *etw.SessionConfiguration) {})
	if err != nil {
		return nil, err
	}
	// Microsoft-Windows-HttpService  {dd5ef90a-6398-47a4-ad34-4dcecdef795f}
	//     https://github.com/repnz/etw-providers-docs/blob/master/Manifests-Win10-18990/Microsoft-Windows-HttpService.xml
	ei.httpguid, err = windows.GUIDFromString("{dd5ef90a-6398-47a4-ad34-4dcecdef795f}")
	if err != nil {
		return nil, fmt.Errorf("Error creating GUID for HTTPService ETW provider: %v", err)
	}
	pidsList := []uint32{0}
	ei.session.ConfigureProvider(ei.httpguid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_INFORMATION
		cfg.PIDs = pidsList
		cfg.MatchAnyKeyword = 0x136
	})
	err = ei.session.EnableProvider(ei.httpguid)
	if err != nil {
		return nil, fmt.Errorf("Error enabling HTTPService ETW provider: %v", err)
	}
	return ei, nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (hei *EtwInterface) SetCapturedProtocols(http, https bool) {
	hei.captureHTTP = http
	hei.captureHTTPS = https
	SetEnabledProtocols(http, https)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (hei *EtwInterface) SetMaxFlows(maxFlows uint64) {
	log.Debugf("Setting max flows in ETW http source to %v", maxFlows)
	SetMaxFlows(maxFlows)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (hei *EtwInterface) SetMaxRequestBytes(maxRequestBytes uint64) {
	log.Debugf("Setting max request bytes in ETW http source to to %v", maxRequestBytes)
	SetMaxRequestBytes(maxRequestBytes)
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (hei *EtwInterface) StartReadingHttpFlows() {
	hei.OnStart()
	hei.eventLoopWG.Add(2)

	startingEtwChan := make(chan struct{})

	// Currently ETW needs be started on a separate thread
	// because it is blocked until subscription is stopped
	go func() {
		defer hei.eventLoopWG.Done()
		startingEtwChan <- struct{}{}
		err := hei.session.StartTracing(func(e *etw.DDEventRecord) {
			// By default this function call never exits and its callbacks or rather events
			// will be returned on the very the same thread until ETW is canceled via
			// etw.StopEtw(). There is asynchronous flag which implicitly will create a real
			// (Windows API) thread but it is not tested yet.
			hei.OnEvent(e)
		})

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

//nolint:revive // TODO(WKIT) Fix revive linter
func (hei *EtwInterface) Close() {
	hei.OnStop()
	if hei.session != nil {
		_ = hei.session.StopTracing()
		hei.eventLoopWG.Wait()
	}
	close(hei.DataChannel)
}

func getRelatedActivityID(e *etw.DDEventRecord) *etw.DDGUID {

	if e.ExtendedDataCount == 0 || e.ExtendedData == nil {
		return nil
	}
	exDatas := unsafe.Slice(e.ExtendedData, e.ExtendedDataCount)
	for _, exData := range exDatas {
		var g etw.DDGUID
		if exData.ExtType == etw.EVENT_HEADER_EXT_TYPE_RELATED_ACTIVITYID && exData.DataSize == uint16(unsafe.Sizeof(g)) {
			activityID := (*etw.DDGUID)(unsafe.Pointer(exData.DataPtr))
			return activityID
		}
	}
	return nil
}

// FormatGUID converts a guid structure to a go string
func FormatGUID(guid etw.DDGUID) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X%02X%02X%02X%02X%02X%02X}",
		guid.Data1, guid.Data2, guid.Data3,
		guid.Data4[0], guid.Data4[1], guid.Data4[2], guid.Data4[3],
		guid.Data4[4], guid.Data4[5], guid.Data4[6], guid.Data4[7])
}
