// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"os"
	"time"

	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/dynamicmapper"
	"github.com/spf13/pflag"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
)

var options *server.CustomMetricsAdapterServerOptions
var stopCh chan struct{}

func init() {
	// FIXME: log to seelog
	options = server.NewCustomMetricsAdapterServerOptions(os.Stdout, os.Stdout)
}

// AddFlags ensures the required flags exist
func AddFlags(fs *pflag.FlagSet) {
	options.SecureServing.AddFlags(fs)
	options.Authentication.AddFlags(fs)
	options.Authorization.AddFlags(fs)
	options.Features.AddFlags(fs)
}

// ValidateArgs validates the custom metrics arguments passed
func ValidateArgs(args []string) error {
	return options.Validate(args)
}

// StartServer creates and start a k8s custom metrics API server
func StartServer() error {
	config, err := options.Config()
	if err != nil {
		return err
	}
	var clientConfig *rest.Config
	clientConfig, err = rest.InClusterConfig()
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to construct discovery client for dynamic client: %v", err)
	}

	dynamicMapper, err := dynamicmapper.NewRESTMapper(discoveryClient, apimeta.InterfacesForUnstructured, time.Second*5)
	if err != nil {
		return fmt.Errorf("unable to construct dynamic discovery mapper: %v", err)
	}

	clientPool := dynamic.NewClientPool(clientConfig, dynamicMapper, dynamic.LegacyAPIPathResolverFunc)
	if err != nil {
		return fmt.Errorf("unable to construct lister client to initialize provider: %v", err)
	}

	client, err := as.GetAPIClient()
	if err != nil {
		return err
	}
	datadogHPAConfigMap := custommetrics.GetHPAConfigmapName()
	store, err := custommetrics.NewConfigMapStore(client.Cl, as.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		return err
	}

	datadogCl, err := hpa.NewDatadogClient()
	if err != nil {
		return err
	}

	// HPA watcher
	hpaClient, err := hpa.NewHPAWatcherClient(client.Cl, datadogCl, store)
	if err != nil {
		return err
	}
	hpaClient.Start()

	emProvider := custommetrics.NewDatadogProvider(clientPool, dynamicMapper, store)
	// As the Custom Metrics Provider is introduced, change the first emProvider to a cmProvider.
	server, err := config.Complete().New("datadog-custom-metrics-adapter", emProvider, emProvider)
	if err != nil {
		return err
	}
	stopCh = make(chan struct{})
	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if stopCh != nil {
		close(stopCh)
	}
}
