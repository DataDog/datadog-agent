// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"context"
	"strconv"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/patrickmn/go-cache"
	"google.golang.org/grpc/metadata"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/proto"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/telemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ClientProvider defines the interface for providing the client connection
type ClientProvider interface {
	GetClient() (pb.AgentSecureClient, error)
	GetContext() context.Context
	GetAuthToken() string
}

// RemoteProgram represents a remote workload filter program.
type RemoteProgram struct {
	Name           string
	ObjectType     string
	Logger         log.Component
	TelemetryStore *telemetry.Store
	Provider       ClientProvider
	Cache          *cache.Cache
}

var _ FilterProgram = &RemoteProgram{}

// Evaluate evaluates the filter program against a workload entity.
// Note: we are assuming a strong dependency on the remote server.
// If the remote server is not available, we consider the entity excluded by default.
func (p *RemoteProgram) Evaluate(entity workloadfilter.Filterable) workloadfilter.Result {
	cacheKey := p.getCacheKey(entity)
	if p.Cache != nil && cacheKey != "" {
		if val, found := p.Cache.Get(cacheKey); found {
			if res, ok := val.(workloadfilter.Result); ok {
				// Slide the expiration
				p.Cache.Set(cacheKey, res, 0)
				p.TelemetryStore.CacheHits.Inc(p.ObjectType, p.Name, res.String())
				return res
			}
		}
		p.TelemetryStore.CacheMisses.Inc(p.ObjectType, p.Name)
	}

	req, err := proto.NewEvaluateRequest(p.Name, entity)
	if err != nil {
		p.Logger.Errorf("unable to build workloadfilter request: %v", err)
		p.TelemetryStore.RemoteEvaluationErrors.Inc(p.ObjectType, p.Name, "request_build_error")
		return workloadfilter.Excluded
	}

	client, err := p.Provider.GetClient()
	if err != nil {
		p.Logger.Errorf("unable to get client: %v", err)
		p.TelemetryStore.RemoteEvaluationErrors.Inc(p.ObjectType, p.Name, "client_error")
		return workloadfilter.Excluded
	}

	// Create the context with the auth token
	queryCtx, queryCancel := context.WithTimeout(
		metadata.NewOutgoingContext(p.Provider.GetContext(), metadata.MD{
			"authorization": []string{"Bearer " + p.Provider.GetAuthToken()},
		}),
		1*time.Second,
	)
	defer queryCancel()

	resp, err := client.WorkloadFilterEvaluate(queryCtx, req)
	if err != nil {
		p.Logger.Errorf("workloadfilter remote evaluation failed: %v", err)
		p.TelemetryStore.RemoteEvaluationErrors.Inc(p.ObjectType, p.Name, "rpc_error")
		return workloadfilter.Excluded
	}

	result := proto.ToWorkloadFilterResult(resp.Result)
	p.TelemetryStore.RemoteEvaluations.Inc(p.ObjectType, p.Name, result.String())

	if p.Cache != nil && cacheKey != "" {
		p.Cache.Set(cacheKey, result, 30*time.Second)
	}

	return result
}

func (p *RemoteProgram) getCacheKey(entity workloadfilter.Filterable) string {
	bytes, err := entity.ToBytes()
	if err != nil {
		p.Logger.Warnf("failed to convert entity to bytes: %v", err)
		return ""
	}
	hash := xxhash.Sum64(bytes)
	return strconv.FormatUint(hash, 16)
}

// GetInitializationErrors returns the initialization errors encountered while creating the program.
func (p *RemoteProgram) GetInitializationErrors() []error {
	return nil
}
