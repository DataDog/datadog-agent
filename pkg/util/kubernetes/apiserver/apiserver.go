// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalApiClient *APIClient

	ErrNotFound = errors.New("entity not found")
	ErrOutdated = errors.New("entity is outdated")
)

const (
	configMapDCAToken = "datadogtoken"
	defaultNamespace  = "default"
	tokenTime         = "tokenTimestamp"
	tokenKey          = "tokenKey"
	servicesPollIntl  = 20 * time.Second
	serviceMapExpire  = 5 * time.Minute
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
			var k8sConfig *k8s.Config
			k8sConfig, err = ParseKubeConfig(cfgPath)
			if err != nil {
				return err
			}
			c.client, err = k8s.NewClient(k8sConfig)
		}
		if err != nil {
			return err
		}
	}

	// Try to get apiserver version to confim connectivity
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	version, err := c.client.Discovery().Version(ctx)
	if err != nil {
		return err
	}

	log.Debugf("Connected to kubernetes apiserver, version %s", version.GitVersion)

	err = c.checkResourcesAuth()
	if err != nil {
		return err
	}
	log.Debug("Could successfully collect Pods, Nodes, Services and Events.")

	useServiceMapper := config.Datadog.GetBool("use_service_mapper")
	if !useServiceMapper {
		return nil
	}
	c.startServiceMapping()
	return nil
}

// ServiceMapperBundle maps the podNames to the serviceNames they are associated with.
// It is updated by mapServices in services.go
type ServiceMapperBundle struct {
	PodNameToServices map[string][]string
	m                 sync.RWMutex
}

func newServiceMapperBundle() *ServiceMapperBundle {
	return &ServiceMapperBundle{
		PodNameToServices: make(map[string][]string),
	}
}

// FetchAndStoreServiceMapping queries the Kubernetes apiserver to get the following resources:
// - node(s), let nodeName as "" to get all nodes (TODO remove when we have the DCA)
// - all endpoints of all namespaces
// - all pods of all namespaces
// Then it stores in cache the ServiceMapperBundle of each node.
func (c *APIClient) FetchAndStoreServiceMapping(nodeName string) error {
	var err error
	// The timeout for the context is the same as the poll frequency.
	// We use a new context at each run, to recover if we can't access the API server temporarily.
	// A poll run should take less than the poll frequency.
	ctx, cancel := context.WithTimeout(context.Background(), servicesPollIntl)
	defer cancel()

	var nodeList *v1.NodeList
	// TODO remove the else part when we have the DCA
	if nodeName == "" {
		// We fetch nodes to reliably use nodename as key in the cache. Avoiding to retrieve them from the endpoints/pods.
		nodeList, err = c.client.CoreV1().ListNodes(ctx)
		if err != nil {
			log.Errorf("Could not collect all nodes from the API Server: %q", err.Error())
			return err
		}
	} else {
		oneNode, err := c.client.CoreV1().GetNode(ctx, nodeName)
		if err != nil {
			log.Errorf("Could not collect node %s from the API Server: %q", nodeName, err.Error())
			return err
		}
		nodeList = &v1.NodeList{Items: []*v1.Node{oneNode}}
	}

	endpointList, err := c.client.CoreV1().ListEndpoints(ctx, "")
	if err != nil {
		log.Errorf("Could not collect endpoints from the API Server: %q", err.Error())
		return err
	}
	if endpointList.Items == nil {
		log.Debug("No services collected from the API server")
		return err
	}
	pods, err := c.client.CoreV1().ListPods(ctx, k8s.AllNamespaces)
	if err != nil {
		log.Errorf("Could not collect pods from the API Server: %q", err.Error())
		return err
	}

	for _, node := range nodeList.Items {
		nodeName := *node.Metadata.Name
		smb, found := cache.Cache.Get(nodeName)
		if !found {
			smb = newServiceMapperBundle()
		}
		err := smb.(*ServiceMapperBundle).mapServices(nodeName, *pods, *endpointList)
		if err != nil {
			log.Errorf("Could not map the services: %s on node %s", err.Error(), nodeName)
			continue
		}
		cache.Cache.Set(nodeName, smb, serviceMapExpire)
	}
	return nil
}

// startServiceMapping is only called once, when we have confirmed we could correctly connect to the API server.
// The logic here is solely to retrieve Nodes, Pods and Endpoints. The processing part is in mapServices.
func (c *APIClient) startServiceMapping() {
	tickerSvcProcess := time.NewTicker(servicesPollIntl)
	go func() {
		for {
			select {
			case <-tickerSvcProcess.C:
				c.FetchAndStoreServiceMapping("")
			}
		}
	}()
}

func aggregateCheckResourcesErrors(errorMessages []string) error {
	if len(errorMessages) == 0 {
		return nil
	}
	return fmt.Errorf("check resources failed: %s", strings.Join(errorMessages, ", "))
}

// checkResourcesAuth is meant to check that we can query resources from the API server.
// Depending on the user's config we only trigger an error if necessary.
// The Event check requires getting Events data.
// The ServiceMapper case, requires access to Services, Nodes and Pods.
func (c *APIClient) checkResourcesAuth() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	var errorMessages []string

	// We always want to collect events
	_, err := c.client.CoreV1().ListEvents(ctx, "")
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("event collection: %q", err.Error()))
	}

	if config.Datadog.GetBool("use_service_mapper") == false {
		return aggregateCheckResourcesErrors(errorMessages)
	}
	_, err = c.client.CoreV1().ListServices(ctx, "")
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("service collection: %q", err.Error()))
	}
	_, err = c.client.CoreV1().ListPods(ctx, "")
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("pod collection: %q", err.Error()))
	}
	_, err = c.client.CoreV1().ListNodes(ctx)
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("node collection: %q", err.Error()))
	}
	return aggregateCheckResourcesErrors(errorMessages)
}

// ParseKubeConfig reads and unmarcshals a kubeconfig file
// in an object ready to use. Exported for integration testing.
func ParseKubeConfig(fpath string) (*k8s.Config, error) {
	// TODO: support yaml too
	jsonFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, err
	}

	k8sConf := &k8s.Config{}
	err = json.Unmarshal(jsonFile, k8sConf)
	return k8sConf, err
}

// ComponentStatuses returns the component status list from the APIServer
func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	return c.client.CoreV1().ListComponentStatuses(ctx)
}

// GetTokenFromConfigmap returns the value of the `tokenValue` from the `tokenKey` in the ConfigMap `configMapDCAToken` if its timestamp is less than tokenTimeout old.
func (c *APIClient) GetTokenFromConfigmap(token string, tokenTimeout int64) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		log.Debugf("Could not find the ConfigMap %s: %s", configMapDCAToken, err.Error())
		return "", false, ErrNotFound
	}
	log.Infof("Found the ConfigMap %s", configMapDCAToken)

	tokenValue, found := tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenKey)]
	if !found {
		log.Errorf("%s was not found in the ConfigMap %s", token, configMapDCAToken)
		return "", found, ErrNotFound
	}
	log.Tracef("%s is %s", token, tokenValue)

	tokenTimeStr, set := tokenConfigMap.Data[fmt.Sprintf("%s.%s", token, tokenTime)] // This is so we can have one timestamp per token

	if !set {
		log.Debugf("Could not find timestamp associated with %s in the ConfigMap %s. Refreshing.", token, configMapDCAToken)
		// We return ErrOutdated to reset the tokenValue and its timestamp as token's timestamp was not found.
		return tokenValue, found, ErrOutdated
	}

	tokenTime, err := time.Parse(time.RFC822, tokenTimeStr)
	if err != nil {
		return "", found, log.Errorf("could not convert the timestamp associated with %s from the ConfigMap %s", token, configMapDCAToken)
	}
	tokenAge := time.Now().Unix() - tokenTime.Unix()

	if tokenAge > tokenTimeout {
		log.Debugf("The tokenValue %s is outdated, refreshing the state", token)
		return tokenValue, found, ErrOutdated
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
