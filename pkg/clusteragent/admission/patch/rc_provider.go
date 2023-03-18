// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// remoteConfigProvider consumes tracing configs from RC and delivers them to the patcher
type remoteConfigProvider struct {
	client        *remote.Client
	isLeaderNotif <-chan struct{}
	subscribers   map[TargetObjKind]chan PatchRequest
	clusterName   string
}

var _ patchProvider = &remoteConfigProvider{}

func newRemoteConfigProvider(client *remote.Client, isLeaderNotif <-chan struct{}, clusterName string) (*remoteConfigProvider, error) {
	if client == nil {
		return nil, errors.New("remote config client not initialized")
	}
	return &remoteConfigProvider{
		client:        client,
		isLeaderNotif: isLeaderNotif,
		subscribers:   make(map[TargetObjKind]chan PatchRequest),
		clusterName:   clusterName,
	}, nil
}

func (rcp *remoteConfigProvider) start(stopCh <-chan struct{}) {
	log.Info("Starting remote-config patch provider")
	rcp.client.RegisterAPMTracing(rcp.process)
	rcp.client.Start()
	for {
		select {
		case <-rcp.isLeaderNotif:
			log.Info("Got a leader notification, polling from remote-config")
			rcp.process(rcp.client.APMTracingConfigs())
		case <-stopCh:
			log.Info("Shutting down remote-config patch provider")
			rcp.client.Close()
			return
		}
	}
}

func (rcp *remoteConfigProvider) subscribe(kind TargetObjKind) chan PatchRequest {
	ch := make(chan PatchRequest, 10)
	rcp.subscribers[kind] = ch
	return ch
}

// process is the event handler called by the RC client on config updates
func (rcp *remoteConfigProvider) process(update map[string]state.APMTracingConfig) {
	log.Infof("Got %d updates from remote-config", len(update))
	var valid, invalid float64
	for path, config := range update {
		log.Debugf("Parsing config %s from path %s", config.Config, path)
		var req PatchRequest
		err := json.Unmarshal(config.Config, &req)
		if err != nil {
			invalid++
			log.Errorf("Error while parsing config: %v", err)
			continue
		}
		log.Debugf("Patch request parsed %+v", req)
		if err := req.Validate(rcp.clusterName); err != nil {
			invalid++
			log.Errorf("Skipping invalid patch request: %s", err)
			continue
		}
		if ch, found := rcp.subscribers[req.K8sTarget.Kind]; found {
			valid++
			log.Debugf("Publishing patch request for target %s", req.K8sTarget)
			ch <- req
		}
	}
	metrics.RemoteConfigs.Set(valid)
	metrics.InvalidRemoteConfigs.Set(invalid)
}
