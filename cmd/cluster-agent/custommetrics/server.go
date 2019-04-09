// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"net"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/apiserver"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"context"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var cmd *DatadogMetricsAdapter

var stopCh chan struct{}

type DatadogMetricsAdapter struct {
	basecmd.AdapterBase
}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {
	cmd = &DatadogMetricsAdapter{}
	cmd.Flags()

	provider, err := cmd.makeProviderOrDie()
	if err != nil {
		return err
	}

	// TODO when implementing the custom metrics provider, add cmd.WithCustomMetrics(provider) here
	cmd.WithExternalMetrics(provider)
	cmd.Name = "datadog-custom-metrics-adapter"

	conf, err := cmd.Config()
	if err != nil {
		return err
	}

	server, err := conf.Complete(nil).New(cmd.Name, nil, provider)
	if err != nil {
		return err
	}

	return server.GenericAPIServer.PrepareRun().Run(wait.NeverStop)
}

func (a *DatadogMetricsAdapter) makeProviderOrDie() (provider.ExternalMetricsProvider, error) {
	client, err := a.DynamicClient()
	if err != nil {
		log.Infof("Unable to construct dynamic client: %v", err)
		return nil, err
	}
	apiCl, err := as.GetAPIClient()
	if err != nil {
		log.Errorf("Could not build API Client: %v", err)
		return nil, err
	}

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	store, err := custommetrics.NewConfigMapStore(apiCl.Cl, common.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		log.Errorf("Unable to create ConfigMap Store: %v", err)
		return nil, err
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		log.Errorf("Unable to construct discovery REST mapper: %v", err)
		return nil, err
	}

	return custommetrics.NewDatadogProvider(context.Background(), client, mapper, store), nil
}

// Config creates the configuration containing the required parameters to communicate with the APIServer as an APIService
func (o *DatadogMetricsAdapter) Config() (*apiserver.Config, error) {
	o.SecureServing.ServerCert.CertDirectory = "/etc/datadog-agent/certificates"
	o.SecureServing.BindPort = config.Datadog.GetInt("external_metrics_provider.port")

	if err := o.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		log.Errorf("Failed to create self signed AuthN/Z configuration %#v", err)
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)
	serverConfig := genericapiserver.NewConfig(codecs)

	err := o.SecureServing.ApplyTo(serverConfig)
	if err != nil {
		log.Errorf("Error while converting SecureServing type %v", err)
		return nil, err
	}

	// Get the certificates from the extension-apiserver-authentication ConfigMap
	if err := o.Authentication.ApplyTo(&serverConfig.Authentication, serverConfig.SecureServing, nil); err != nil {
		log.Errorf("Could not create Authentication configuration: %v", err)
		return nil, err
	}

	if err := o.Authorization.ApplyTo(&serverConfig.Authorization); err != nil {
		log.Infof("Could not create Authorization configuration: %v", err)
		return nil, err
	}

	return &apiserver.Config{
		GenericConfig: serverConfig,
	}, nil
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if stopCh != nil {
		close(stopCh)
	}
}
