// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// APIServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type APIServer struct {
	msgs                 chan *api.SecurityEventMessage
	processMsgs          chan *api.SecurityProcessEventMessage
	activityDumps        chan *api.ActivityDumpStreamMessage
	expiredEventsLock    sync.RWMutex
	expiredEvents        map[rules.RuleID]*atomic.Int64
	expiredProcessEvents *atomic.Int64
	expiredDumpsLock     sync.RWMutex
	expiredDumps         *atomic.Int64
	//rate                 *Limiter
	statsdClient statsd.ClientInterface
	probe        *sprobe.Probe
	queueLock    sync.Mutex
	queue        []*pendingMsg
	retention    time.Duration
	cfg          *config.Config
	module       *Module
}

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.Config, probe *sprobe.Probe, client statsd.ClientInterface) *APIServer {
	es := &APIServer{
		msgs:                 make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		processMsgs:          make(chan *api.SecurityProcessEventMessage, cfg.EventServerBurst*3),
		activityDumps:        make(chan *api.ActivityDumpStreamMessage, model.MaxTracedCgroupsCount*2),
		expiredEvents:        make(map[rules.RuleID]*atomic.Int64),
		expiredProcessEvents: atomic.NewInt64(0),
		expiredDumps:         atomic.NewInt64(0),
		//rate:                 NewLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
		statsdClient: client,
		probe:        probe,
		retention:    time.Duration(cfg.EventServerRetention) * time.Second,
		cfg:          cfg,
	}
	return es
}

// DumpActivity handle an activity dump request
func (a *APIServer) DumpActivity(ctx context.Context, params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	/*
		if monitor := a.probe.GetMonitor(); monitor != nil {
			msg, err := monitor.DumpActivity(params)
			if err != nil {
				seclog.Errorf(err.Error())
			}
			return msg, nil
		}
	*/
	return nil, nil //fmt.Errorf("monitor not configured")
}

// DumpDiscarders handles discarder dump requests
func (a *APIServer) DumpDiscarders(ctx context.Context, params *api.DumpDiscardersParams) (*api.DumpDiscardersMessage, error) {
	/*
		filePath, err := a.probe.DumpDiscarders()
		if err != nil {
			return nil, err
		}
		seclog.Infof("Discarder dump file path: %s", filePath)

		return &api.DumpDiscardersMessage{DumpFilename: filePath}, nil
	*/
	return nil, nil
}

// DumpNetworkNamespace handles network namespace cache dump requests
func (a *APIServer) DumpNetworkNamespace(ctx context.Context, params *api.DumpNetworkNamespaceParams) (*api.DumpNetworkNamespaceMessage, error) {
	//return a.probe.GetResolvers().NamespaceResolver.DumpNetworkNamespaces(params), nil
	return nil, nil
}

// DumpProcessCache handles process cache dump requests
func (a *APIServer) DumpProcessCache(ctx context.Context, params *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	/*
		resolvers := a.probe.GetResolvers()

		filename, err := resolvers.ProcessResolver.Dump(params.WithArgs)
		if err != nil {
			return nil, err
		}

		return &api.SecurityDumpProcessCacheMessage{
			Filename: filename,
		}, nil
	*/
	return nil, nil
}

// GetEvents waits for security events
func (a *APIServer) GetEvents(params *api.GetEventParams, stream api.SecurityModule_GetEventsServer) error {
	// Read 10 security events per call
	msgs := 10
LOOP:
	for {
		// Check that the limit is not reached
		//if !a.rate.limiter.Allow() {
		//	return nil
		//}

		// Read one message
		select {
		case msg := <-a.msgs:
			if err := stream.Send(msg); err != nil {
				return err
			}
			msgs--
		case <-time.After(time.Second):
			break LOOP
		}

		// Stop the loop when 10 messages were retrieved
		if msgs <= 0 {
			break
		}
	}

	return nil
}

// GetStatus returns the status of the module
func (a *APIServer) GetStatus(ctx context.Context, params *api.GetStatusParams) (*api.Status, error) {
	/*
		status, err := a.probe.GetConstantFetcherStatus()
		if err != nil {
			return nil, err
		}

		constants := make([]*api.ConstantValueAndSource, 0, len(status.Values))
		for _, v := range status.Values {
			constants = append(constants, &api.ConstantValueAndSource{
				ID:     v.ID,
				Value:  v.Value,
				Source: v.FetcherName,
			})
		}

		apiStatus := &api.Status{
			Environment: &api.EnvironmentStatus{
				Constants: &api.ConstantFetcherStatus{
					Fetchers: status.Fetchers,
					Values:   constants,
				},
			},
			SelfTests: a.module.selfTester.GetStatus(),
		}

		envErrors := a.probe.VerifyEnvironment()
		if envErrors != nil {
			apiStatus.Environment.Warnings = make([]string, len(envErrors.Errors))
			for i, err := range envErrors.Errors {
				apiStatus.Environment.Warnings[i] = err.Error()
			}
		}

		apiStatus.Environment.KernelLockdown = string(kernel.GetLockdownMode())

		if kernel, err := a.probe.GetKernelVersion(); err == nil {
			apiStatus.Environment.UseMmapableMaps = kernel.HaveMmapableMaps()
			apiStatus.Environment.UseRingBuffer = a.probe.UseRingBuffers()
		}
	*/
	apiStatus := &api.Status{}
	return apiStatus, nil
}

// ListActivityDumps returns the list of active dumps
func (a *APIServer) ListActivityDumps(ctx context.Context, params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	/*
		if monitor := a.probe.GetMonitor(); monitor != nil {
			msg, err := monitor.ListActivityDumps(params)
			if err != nil {
				seclog.Errorf(err.Error())
			}
			return msg, nil
		}
	*/
	return nil, fmt.Errorf("monitor not configured")
}

// RunSelfTest runs self test and then reload the current policies
func (a *APIServer) RunSelfTest(ctx context.Context, params *api.RunSelfTestParams) (*api.SecuritySelfTestResultMessage, error) {
	/*
		if a.module == nil {
			return nil, errors.New("failed to found module in APIServer")
		}

		if a.module.selfTester == nil {
			return &api.SecuritySelfTestResultMessage{
				Ok:    false,
				Error: "self-tests are disabled",
			}, nil
		}

		if err := a.module.RunSelfTest(false); err != nil {
			return &api.SecuritySelfTestResultMessage{
				Ok:    false,
				Error: err.Error(),
			}, nil
		}
	*/
	return &api.SecuritySelfTestResultMessage{
		Ok:    true,
		Error: "",
	}, nil
}

// StopActivityDump stops an active activity dump if it exists
func (a *APIServer) StopActivityDump(ctx context.Context, params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	/*
		if monitor := a.probe.GetMonitor(); monitor != nil {
			msg, err := monitor.StopActivityDump(params)
			if err != nil {
				seclog.Errorf(err.Error())
			}
			return msg, nil
		}
	*/
	return nil, fmt.Errorf("monitor not configured")
}

// TranscodingRequest encodes an activity dump following the requested parameters
func (a *APIServer) TranscodingRequest(ctx context.Context, params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	/*
		if monitor := a.probe.GetMonitor(); monitor != nil {
			msg, err := monitor.GenerateTranscoding(params)
			if err != nil {
				seclog.Errorf(err.Error())
			}
			return msg, nil
		}
	*/
	return nil, fmt.Errorf("monitor not configured")
}
