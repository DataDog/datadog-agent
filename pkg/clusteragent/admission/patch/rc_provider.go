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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// remoteConfigProvider consumes tracing configs from RC and delivers them to the patcher
type remoteConfigProvider struct {
	client             *rcclient.Client
	isLeaderNotif      <-chan struct{}
	subscribers        map[TargetObjKind]chan Request
	clusterName        string
	telemetryCollector telemetry.TelemetryCollector
	webhook            *autoinstrumentation.Webhook
}

var _ patchProvider = &remoteConfigProvider{}

func newRemoteConfigProvider(client *rcclient.Client, isLeaderNotif <-chan struct{}, telemetryCollector telemetry.TelemetryCollector, clusterName string, webhook *autoinstrumentation.Webhook) (*remoteConfigProvider, error) {
	if client == nil {
		return nil, errors.New("remote config client not initialized")
	}
	return &remoteConfigProvider{
		client:             client,
		isLeaderNotif:      isLeaderNotif,
		subscribers:        make(map[TargetObjKind]chan Request),
		clusterName:        clusterName,
		telemetryCollector: telemetryCollector,
	}, nil
}

func (rcp *remoteConfigProvider) start(stopCh <-chan struct{}) {
	log.Info("Starting remote-config patch provider")
	rcp.client.Subscribe(state.ProductAPMTracing, rcp.process)
	rcp.client.Start()
	for {
		select {
		case <-rcp.isLeaderNotif:
			log.Info("Got a leader notification, polling from remote-config")
			rcp.process(rcp.client.GetConfigs(state.ProductAPMTracing), rcp.client.UpdateApplyStatus)
		case <-stopCh:
			log.Info("Shutting down remote-config patch provider")
			rcp.client.Close()
			return
		}
	}
}

func (rcp *remoteConfigProvider) subscribe(kind TargetObjKind) chan Request {
	ch := make(chan Request, 10)
	rcp.subscribers[kind] = ch
	return ch
}

// process is the event handler called by the RC client on config updates
func (rcp *remoteConfigProvider) process(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
	log.Infof("Got %d updates from remote-config", len(update))
	var valid, invalid float64
	for path, config := range update {
		log.Debugf("Parsing config %s from path %s", config.Config, path)
		var req Request
		err := json.Unmarshal(config.Config, &req)
		if err != nil {
			invalid++
			rcp.telemetryCollector.SendRemoteConfigPatchEvent(req.getApmRemoteConfigEvent(err, telemetry.ConfigParseFailure))
			log.Errorf("Error while parsing config: %v", err)
			continue
		}
		req.RcVersion = config.Metadata.Version
		log.Debugf("Patch request parsed %+v", req)

		// Check for null K8sTargetV2 to be forward compatible
		if req.K8sTargetV2 == nil {
			log.Errorf("Received a patch request without a K8sTargetV2, skipping")
			return
		}

		// Find the configuration for the current cluster
		var clusterTarget *K8sClusterTarget
		for _, ct := range req.K8sTargetV2.ClusterTargets {
			if ct.ClusterName == rcp.clusterName {
				clusterTarget = &ct
				break
			}
		}
		if clusterTarget == nil {
			log.Errorf("Received an APM request without a configuration for the current cluster, skipping")
			return
		}

		// TODO: need to store previous config to revert back to. Also need to handle when remote config deleted.
		if clusterTarget.EnabledNamespaces != nil {
			strEnabledNamespaces := ""
			for _, ns := range *clusterTarget.EnabledNamespaces {
				strEnabledNamespaces += ns + ","
			}
			if len(strEnabledNamespaces) > 0 {
				ddconfig.Datadog.Set("apm_config.instrumentation.enabled_namespaces", strEnabledNamespaces, "remote_config")
			}
		}

		if clusterTarget.Enabled != nil {
			if *clusterTarget.Enabled == false {
				ddconfig.Datadog.Set("apm_config.instrumentation.enabled", "false", "remote_config")
			} else {
				ddconfig.Datadog.Set("apm_config.instrumentation.enabled", "true", "remote_config")
			}
		}

		// Recompute the filter for the webhook
		rcp.webhook.RecomputeFilter()
	}
	metrics.RemoteConfigs.Set(valid)
	metrics.InvalidRemoteConfigs.Set(invalid)
}
