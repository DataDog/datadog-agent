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
	"time"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"

	"github.com/ericchiang/k8s/api/v1"

	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var globalApiClient *APIClient

const (
	configMapDCAToken = "configmapdcatoken"
	defaultNamespace  = "default"
	tokenTime         = "tokenTimestamp"
	tokenKey          = "tokenKey"
)

// ApiClient provides authenticated access to the
// apiserver endpoints. Use the shared instance via GetApiClient.
type APIClient struct {
	// used to setup the APIClient
	initRetry retry.Retrier

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
		globalApiClient.initRetry.SetupRetrier(&retry.Config{
			Name:          "apiserver",
			AttemptMethod: globalApiClient.connect,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	err := globalApiClient.initRetry.TriggerRetry()
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

// GetTokenFromConfigmap returns the value of the `tokenValue` from the `tokenKey` in the ConfigMap configMapDCAToken if its timestamp is less than tokenTimeout old.
func (c *APIClient) GetTokenFromConfigmap(token string, tokenTimeout int64) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		log.Debugf("Could not find the ConfigMap %s: %s", configMapDCAToken, err.Error())
		return "", false, collectors.ErrNotFound
	}
	log.Infof("Found the ConfigMap %s", configMapDCAToken)

	tokenValue, found := tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenKey)]
	if !found {
		log.Errorf("%s was not found in the ConfigMap %s", token, configMapDCAToken)
		return "", found, collectors.ErrNotFound
	}
	log.Tracef("%s is %s", token, tokenValue)

	tokenTimeStr, set := tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenTime)] // This is so we can have one timestamp per token

	if !set {
		log.Debugf("Could not find timestamp associated with %s in the ConfigMap %s. Refreshing.", token, configMapDCAToken)
		// We return ErrOutdated to reset the tokenValue and its timestamp as token's timestamp was not found.
		return tokenValue, found, collectors.ErrOutdated
	}

	tokenTime, err := time.Parse(time.RFC822, tokenTimeStr)
	if err != nil {
		return "", found, log.Errorf("could not convert the timestamp associated with %s from the ConfigMap %s", token, configMapDCAToken)
	}
	tokenAge := time.Now().Unix() - tokenTime.Unix()

	if tokenAge > tokenTimeout {
		log.Debugf("The tokenValue %s is outdated, refreshing the state", token)
		return tokenValue, found, collectors.ErrOutdated
	}
	log.Debugf("Token %s was updated recently, using value to collect newer events.", token)
	return tokenValue, found, nil
}

// UpdateTokenInConfigmap updates the value of the `tokenValue` from the `tokenKey` and sets its collected timestamp in the ConfigMap `configmaptokendca`
func (c *APIClient) UpdateTokenInConfigmap(token, tokenValue string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		return err
	}
	tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenKey)] = tokenValue
	now := time.Now()
	tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenTime)] = now.Format(time.RFC822) // Timestamps in the ConfigMap should all use the type int.

	_, err = c.client.CoreV1().UpdateConfigMap(ctx, tokenConfigMap)
	if err != nil {
		return err
	}
	log.Debugf("Updated %s to %s in the ConfigMap %s", token, tokenValue, configMapDCAToken)
	return nil
}
