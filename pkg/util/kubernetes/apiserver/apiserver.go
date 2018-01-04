// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"context"
	"io/ioutil"
	"time"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/ericchiang/k8s/api/v1"
)

var globalApiClient *APIClient

// ApiClient provides authenticated access to the
// apiserver endpoints. Use the shared instance via GetApiClient.
type APIClient struct {
	retry.Retrier
	client  *k8s.Client
	timeout time.Duration
}

// GetAPIClient returns the shared ApiClient instance.
func GetAPIClient() (*APIClient, error) {
	if globalApiClient == nil {
		globalApiClient = &APIClient{
			// TODO: make it configurable if requested
			timeout: 5 * time.Second,
		}
		globalApiClient.SetupRetrier(&retry.Config{
			Name:          "apiserver",
			AttemptMethod: globalApiClient.connect,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	err := globalApiClient.TriggerRetry()
	if err != nil {
		log.Debugf("init error: %s", err)
		return nil, err
	}
	return globalApiClient, nil
}

func (c *APIClient) connect() error {
	if c.client == nil {
		var err error
		cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
		if cfgPath == "" {
			// Autoconfiguration
			log.Debugf("using autoconfiguration")
			c.client, err = k8s.NewInClusterClient()
		} else {
			// Kubeconfig provided by conf
			log.Debugf("using credentials from %s", cfgPath)
			var config *k8s.Config
			config, err = parseKubeConfig(cfgPath)
			if err != nil {
				return err
			}
			c.client, err = k8s.NewClient(config)
		}
		if err != nil {
			return err
		}
	}

	// Try to get apiserver version to confim connectivity
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	version, err := c.client.Discovery().Version(ctx)
	if err == nil {
		log.Debugf("connected to kubernetes apiserver, version %s", version.GitVersion)
	}
	return err
}

func parseKubeConfig(fpath string) (*k8s.Config, error) {
	yamlFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, err
	}

	config := &k8s.Config{}
	err = yaml.Unmarshal(yamlFile, config)
	return config, err
}

func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	return c.client.CoreV1().ListComponentStatuses(ctx)

}
