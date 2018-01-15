// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"strconv"
	"time"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"

	"github.com/ericchiang/k8s/api/v1"

	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var globalApiClient *APIClient

const (
	configMapDCAToken = "configmaptokendca"
	defaultNamespace  = "default"
	tokenTime         = "tokenTimestamp"
	tokenTimeout      = 60
)

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
			config, err = ParseKubeConfig(cfgPath)
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

// ParseKubeConfig reads and unmarcshals a kubeconfig file
// in an object ready to use. Exported for integration testing.
func ParseKubeConfig(fpath string) (*k8s.Config, error) {
	// TODO: support yaml too
	jsonFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, err
	}

	config := &k8s.Config{}
	err = json.Unmarshal(jsonFile, config)
	return config, err
}

// ComponentStatuses returns the component status list from the APIServer
func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	return c.client.CoreV1().ListComponentStatuses(ctx)
}

// ConfigMapTokenFetcher returns the value of the `tokenValue` from the `tokenKey` in the ConfigMap `configmaptokendca` if its timestamp is less than tokenTimeout old.
func (c *APIClient) ConfigMapTokenFetcher(tokenKey string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		return "", false, err
	}
	log.Infof("Found the ConfigMap %s", configMapDCAToken)

	token, found := tokenConfigMap.Data[fmt.Sprintf("%s.Key", tokenKey)]
	if !found {
		return "", found, log.Errorf("%s was not found in the ConfigMap %s", tokenKey, configMapDCAToken)
	}
	log.Debugf("%s is %s", tokenKey, token)

	tokenTimeStr, set := tokenConfigMap.Data[fmt.Sprintf("%s.%s", tokenKey, tokenTime)] // This is so we can have one timestamp per tokenKey

	if set {
		tokenTime, err := time.Parse(time.RFC822, tokenTimeStr)
		if err != nil {
			return "", found, log.Errorf("could not convert the timestamp associated with %s from the ConfigMap %s", tokenKey, configMapDCAToken)
		}
		tokenAge := time.Now().Unix() - tokenTime.Unix()

		if tokenAge > tokenTimeout {
			log.Infof("The event token is outdated, refreshing the events")
			return "", found, nil
		}
	}

	log.Debugf("Could not find timestamp associated with %s in the ConfigMap %s. Refreshing.", tokenKey, configMapDCAToken)

	// We return token = "" to reset the token and its timestamp as tokenKey's timestamp was not found.
	return "", found, log.Errorf("could not find %s in the ConfigMap %s", tokenKey, configMapDCAToken)
}

//EventTokenSetter updates the value of the `tokenValue` from the `tokenKey` and sets its collected timestamp in the ConfigMap `configmaptokendca`
func (c *APIClient) ConfigMapTokenSetter(tokenKey, tokenValue string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		return err
	}
	tokenConfigMap.Data[tokenKey] = tokenValue
	now := time.Now()
	tokenConfigMap.Data[fmt.Sprintf("%s.%s", tokenKey, tokenTime)] = now.Format(time.RFC822) // Timestamps in the ConfigMap should all use the type int.

	_, err = c.client.CoreV1().UpdateConfigMap(ctx, tokenConfigMap)
	if err != nil {
		return err
	}
	log.Debugf("Updated %s to %s in the ConfigMap %s", tokenKey, tokenValue, configMapDCAToken)
	return nil
}
