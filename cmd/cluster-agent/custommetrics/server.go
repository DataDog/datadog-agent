// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"context"
	"fmt"
	"net"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/apiserver"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
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

// RunServer creates and start a k8s custom metrics API server
func RunServer(ctx context.Context) error {
	defer clearServerResources()
	cmd = &DatadogMetricsAdapter{}
	cmd.Flags()

	provider, err := cmd.makeProviderOrDie(ctx)
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
	// TODO Add extra logic to only tear down the External Metrics Server if only some components fail.
	return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
}

func (a *DatadogMetricsAdapter) makeProviderOrDie(ctx context.Context) (provider.ExternalMetricsProvider, error) {
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

	mapper, err := a.RESTMapper()
	if err != nil {
		log.Errorf("Unable to construct discovery REST mapper: %v", err)
		return nil, err
	}

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		return externalmetrics.NewDatadogMetricProvider(ctx, apiCl)
	} else {
		datadogHPAConfigMap := custommetrics.GetConfigmapName()
		store, err := custommetrics.NewConfigMapStore(apiCl.Cl, common.GetResourcesNamespace(), datadogHPAConfigMap)
		if err != nil {
			log.Errorf("Unable to create ConfigMap Store: %v", err)
			return nil, err
		}

		return custommetrics.NewDatadogProvider(ctx, client, mapper, store), nil
	}
}

// Config creates the configuration containing the required parameters to communicate with the APIServer as an APIService
func (a *DatadogMetricsAdapter) Config() (*apiserver.Config, error) {
	a.SecureServing.ServerCert.CertDirectory = "/etc/datadog-agent/certificates"
	a.SecureServing.BindPort = config.Datadog.GetInt("external_metrics_provider.port")

	if err := a.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		log.Errorf("Failed to create self signed AuthN/Z configuration %#v", err)
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
	serverConfig := genericapiserver.NewConfig(codecs)

	err := a.SecureServing.ApplyTo(&serverConfig.SecureServing, &serverConfig.LoopbackClientConfig)
	if err != nil {
		log.Errorf("Error while converting SecureServing type %v", err)
		return nil, err
	}

	// Get the certificates from the extension-apiserver-authentication ConfigMap
	if err := a.Authentication.ApplyTo(&serverConfig.Authentication, serverConfig.SecureServing, nil); err != nil {
		log.Errorf("Could not create Authentication configuration: %v", err)
		return nil, err
	}

	if err := a.Authorization.ApplyTo(&serverConfig.Authorization); err != nil {
		log.Infof("Could not create Authorization configuration: %v", err)
		return nil, err
	}

	return &apiserver.Config{
		GenericConfig: serverConfig,
	}, nil
}

// clearServerResources closes the connection and the server
// stops listening to new commands.
func clearServerResources() {
	if stopCh != nil {
		close(stopCh)
	}
}
