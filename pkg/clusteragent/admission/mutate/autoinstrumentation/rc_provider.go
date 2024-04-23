// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"errors"
	"time"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// remoteConfigProvider consumes tracing configs from RC and delivers them to the patcher
type remoteConfigProvider struct {
	client                  *rcclient.Client
	pollInterval            time.Duration
	subscribers             map[TargetObjKind]chan Request
	responseChan            chan Response
	clusterName             string
	lastProcessedRCRevision int64
	rcConfigIDs             map[string]struct{}

	cache *instrumentationConfigurationCache
}

type rcProvider interface {
	start(stopCh <-chan struct{})
}

var _ rcProvider = &remoteConfigProvider{}

func newRemoteConfigProvider(
	client *rcclient.Client,
	clusterName string,
	cache *instrumentationConfigurationCache,
) (*remoteConfigProvider, error) {
	if client == nil {
		return nil, errors.New("remote config client not initialized")
	}
	return &remoteConfigProvider{
		client:                  client,
		subscribers:             make(map[TargetObjKind]chan Request),
		responseChan:            make(chan Response, 10),
		clusterName:             clusterName,
		pollInterval:            10 * time.Second,
		lastProcessedRCRevision: 0,
		rcConfigIDs:             make(map[string]struct{}),
		cache:                   cache,
	}, nil
}

func (rcp *remoteConfigProvider) start(stopCh <-chan struct{}) {
	log.Info("Remote Enablement: starting remote-config provider")
	rcp.client.Subscribe(state.ProductAPMTracing, rcp.process)
	rcp.client.Start()
	ticker := time.NewTicker(rcp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Info("Remote Enablement: polling configuration from remote-config")
			rcp.process(rcp.client.GetConfigs(state.ProductAPMTracing), rcp.client.UpdateApplyStatus)
		case <-stopCh:
			log.Info("Remote Enablement: shutting down remote-config patch provider")
			rcp.client.Close()
			return
		}
	}
}

// process is the event handler called by the RC client on config updates
func (rcp *remoteConfigProvider) process(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Infof("Got %d updates from remote-config", len(update))
	toDelete := make(map[string]struct{}, len(rcp.rcConfigIDs))
	for k, _ := range rcp.rcConfigIDs {
		toDelete[k] = struct{}{}
	}

	for path, config := range update {
		log.Debugf("Parsing config %s from path %s", config.Config, path)
		var req Request
		err := json.Unmarshal(config.Config, &req)
		if err != nil {
			log.Errorf("Error while parsing config: %v", err)
			continue
		}

		if _, ok := toDelete[req.ID]; ok {
			delete(toDelete, req.ID)
		} else {
			rcp.rcConfigIDs[req.ID] = struct{}{}
		}

		if shouldSkipConfig(req, rcp.lastProcessedRCRevision, rcp.clusterName) {
			continue
		}

		req.RcVersion = config.Metadata.Version
		log.Debugf("Patch request parsed %+v", req)
		resp := rcp.cache.update(req)
		applyStateCallback(path, resp.Status)
		rcp.lastProcessedRCRevision = req.Revision
	}

	for configToDelete := range toDelete {
		log.Infof("Remote Enablement: deleting config %s", configToDelete)
		if err := rcp.cache.delete(configToDelete); err != nil {
			log.Errorf("Remote Enablement: failed to delete config %s with %v", configToDelete, err)
		}
	}
}

func shouldSkipConfig(req Request, lastAppliedRevision int64, clusterName string) bool {
	// check if config should be applied based on presence K8sTargetV2 object
	if req.K8sTargetV2 == nil || len(req.K8sTargetV2.ClusterTargets) == 0 {
		log.Debugf("Remote Enablement: skipping config %s because K8sTargetV2 is not set", req.ID)
		return true
	}

	// check if config should be applied based on RC revision
	lastAppliedTime := time.UnixMilli(lastAppliedRevision)
	requestTime := time.UnixMilli(req.Revision)

	if requestTime.Before(lastAppliedTime) || requestTime.Equal(lastAppliedTime) {
		log.Debugf("Remote Enablement: skipping config %s because it has already been applied: revision %v, last applied revision %v", req.ID, requestTime, lastAppliedTime)
		return true
	}

	isTargetingCluster := false
	for _, target := range req.K8sTargetV2.ClusterTargets {
		if target.ClusterName == clusterName {
			isTargetingCluster = true
			break
		}
	}
	if !isTargetingCluster {
		log.Debugf("Remote Enablement: skipping config %s because it's not targeting current cluster %s", req.ID, req.K8sTargetV2.ClusterTargets[0].ClusterName)
	}
	return !isTargetingCluster

}
