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
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalApiClient *APIClient

	ErrNotFound  = errors.New("entity not found")
	ErrOutdated  = errors.New("entity is outdated")
	ErrNotLeader = errors.New("not Leader")
)

const (
	configMapDCAToken         = "datadogtoken"
	defaultNamespace          = "default"
	tokenTime                 = "tokenTimestamp"
	tokenKey                  = "tokenKey"
	metadataPollIntl          = 20 * time.Second
	metadataMapExpire         = 5 * time.Minute
	metadataMapperCachePrefix = "KubernetesMetadataMapping"
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
		log.Debugf("Cannot get the version: %s ", err)
		return err
	}

	log.Debugf("Connected to kubernetes apiserver, version %s", version.GitVersion)

	err = c.checkResourcesAuth()
	if err != nil {
		return err
	}
	log.Debug("Could successfully collect Pods, Nodes, Services and Events.")

	useMetadataMapper := config.Datadog.GetBool("use_metadata_mapper")
	if !useMetadataMapper {
		return nil
	}

	return nil
}

// MetadataMapperBundle maps the podNames to the metadata they are associated with.
// ex: services.
// It is updated by mapServices in services.go
// example: [ "pod" : ["svc1","svc2"]]
type MetadataMapperBundle struct {
	PodNameToService map[string][]string `json:"services,omitempty"`
	m                sync.RWMutex
}

func newMetadataMapperBundle() *MetadataMapperBundle {
	return &MetadataMapperBundle{
		PodNameToService: make(map[string][]string),
	}
}

// NodeMetadataMapping only fetch the endpoints from Kubernetes apiserver and add the metadataMapper of the
// node to the cache
// Only called when the node agent computes the metadata mapper locally and does not rely on the DCA.
func (c *APIClient) NodeMetadataMapping(nodeName string, podList *v1.PodList) error {
	ctx, cancel := context.WithTimeout(context.Background(), metadataPollIntl)
	defer cancel()

	endpointList, err := c.client.CoreV1().ListEndpoints(ctx, "")
	if err != nil {
		log.Errorf("Could not collect endpoints from the API Server: %q", err.Error())
		return err
	}
	if endpointList.Items == nil {
		log.Debug("No endpoints collected from the API server")
		return nil
	}
	log.Debugf("Successfully collected endpoints")

	node := &v1.Node{Metadata: &metav1.ObjectMeta{Name: &nodeName}}
	nodeList := &v1.NodeList{
		Items: []*v1.Node{
			node,
		},
	}
	processKubeServices(nodeList, podList, endpointList)
	return nil
}

// ClusterMetadataMapping queries the Kubernetes apiserver to get the following resources:
// - all nodes
// - all endpoints of all namespaces
// - all pods of all namespaces
// Then it stores in cache the MetadataMapperBundle of each node.
func (c *APIClient) ClusterMetadataMapping() error {
	// The timeout for the context is the same as the poll frequency.
	// We use a new context at each run, to recover if we can't access the API server temporarily.
	// A poll run should take less than the poll frequency.
	ctx, cancel := context.WithTimeout(context.Background(), metadataPollIntl)
	defer cancel()

	// We fetch nodes to reliably use nodename as key in the cache.
	// Avoiding to retrieve them from the endpoints/podList.
	nodeList, err := c.client.CoreV1().ListNodes(ctx)
	if err != nil {
		log.Errorf("Could not collect nodes from the kube-apiserver: %q", err.Error())
		return err
	}
	if nodeList.Items == nil {
		log.Debug("No node collected from the kube-apiserver")
		return nil
	}

	endpointList, err := c.client.CoreV1().ListEndpoints(ctx, k8s.AllNamespaces)
	if err != nil {
		log.Errorf("Could not collect endpoints from the kube-apiserver: %q", err.Error())
		return err
	}
	if endpointList.Items == nil {
		log.Debug("No endpoint collected from the kube-apiserver")
		return nil
	}

	podList, err := c.client.CoreV1().ListPods(ctx, k8s.AllNamespaces)
	if err != nil {
		log.Errorf("Could not collect pods from the kube-apiserver: %q", err.Error())
		return err
	}
	if podList.Items == nil {
		log.Debug("No pod collected from the kube-apiserver")
		return nil
	}

	processKubeServices(nodeList, podList, endpointList)
	return nil
}

// processKubeServices adds services to the metadataMapper cache, pointer parameters must be non nil
func processKubeServices(nodeList *v1.NodeList, podList *v1.PodList, endpointList *v1.EndpointsList) {
	if nodeList.Items == nil || podList.Items == nil || endpointList.Items == nil {
		return
	}
	log.Debugf("Identified: %d node, %d pod, %d endpoints", len(nodeList.Items), len(podList.Items), len(endpointList.Items))
	for _, node := range nodeList.Items {
		nodeName := *node.Metadata.Name
		nodeNameCacheKey := cache.BuildAgentKey(metadataMapperCachePrefix, nodeName)
		metaBundle, found := cache.Cache.Get(nodeNameCacheKey)
		if !found {
			metaBundle = newMetadataMapperBundle()
		}
		err := metaBundle.(*MetadataMapperBundle).mapServices(nodeName, *podList, *endpointList)
		if err != nil {
			log.Errorf("Could not map the services: %s on node %s", err.Error(), *node.Metadata.Name)
			continue
		}
		cache.Cache.Set(nodeNameCacheKey, metaBundle, metadataMapExpire)
	}
}

// StartMetadataMapping is only called once, when we have confirmed we could correctly connect to the API server.
// The logic here is solely to retrieve Nodes, Pods and Endpoints. The processing part is in mapServices.
func (c *APIClient) StartMetadataMapping() {
	tickerSvcProcess := time.NewTicker(metadataPollIntl)
	go func() {
		for {
			select {
			case <-tickerSvcProcess.C:
				c.ClusterMetadataMapping()
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
// The MetadataMapper case, requires access to Services, Nodes and Pods.
func (c *APIClient) checkResourcesAuth() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	var errorMessages []string

	// We always want to collect events
	_, err := c.client.CoreV1().ListEvents(ctx, "")
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("event collection: %q", err.Error()))
	}

	if config.Datadog.GetBool("use_metadata_mapper") == false {
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
	_, err = c.client.CoreV1().ListEndpoints(ctx, "")
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("endpoints collection: %q", err.Error()))
	}
	return aggregateCheckResourcesErrors(errorMessages)
}

// ParseKubeConfig reads and unmarshal a kubeconfig file
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

	eventTokenKey := fmt.Sprintf("%s.%s", token, tokenKey)
	tokenValue, found := tokenConfigMap.Data[eventTokenKey]
	if !found {
		log.Errorf("%s was not found in the ConfigMap %s", eventTokenKey, configMapDCAToken)
		return "", found, ErrNotFound
	}
	log.Infof("%s is %q", token, tokenValue)

	eventTokenTS := fmt.Sprintf("%s.%s", token, tokenTime)
	tokenTimeStr, set := tokenConfigMap.Data[eventTokenTS] // This is so we can have one timestamp per token

	if !set {
		log.Debugf("Could not find timestamp associated with %s in the ConfigMap %s. Refreshing.", eventTokenTS, configMapDCAToken)
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

// UpdateTokenInConfigmap updates the value of the `tokenValue` from the `tokenKey` and
// sets its collected timestamp in the ConfigMap `configmaptokendca`
func (c *APIClient) UpdateTokenInConfigmap(token, tokenValue string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	tokenConfigMap, err := c.client.CoreV1().GetConfigMap(ctx, configMapDCAToken, defaultNamespace)
	if err != nil {
		return err
	}

	eventTokenKey := fmt.Sprintf("%s.%s", token, tokenKey)
	tokenConfigMap.Data[eventTokenKey] = tokenValue

	now := time.Now()
	eventTokenTS := fmt.Sprintf("%s.%s", token, tokenTime)
	tokenConfigMap.Data[eventTokenTS] = now.Format(time.RFC822) // Timestamps in the ConfigMap should all use the type int.

	_, err = c.client.CoreV1().UpdateConfigMap(ctx, tokenConfigMap)
	if err != nil {
		return err
	}
	log.Debugf("Updated %s to %s in the ConfigMap %s", eventTokenKey, tokenValue, configMapDCAToken)
	return nil
}

func (c *APIClient) NodeLabels(nodeName string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	node, err := c.client.CoreV1().GetNode(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	return node.GetMetadata().GetLabels(), nil
}

// GetMetadataMapBundleOnAllNodes is used for the CLI svcmap command to run fetch the metadata map of all nodes.
func GetMetadataMapBundleOnAllNodes() (map[string]interface{}, error) {
	nodePodMetadataMap := make(map[string]*MetadataMapperBundle)
	stats := make(map[string]interface{})
	var warnlist []string
	var warn string
	var err error

	nodes, err := getNodeList()
	if err != nil {
		stats["Errors"] = fmt.Sprintf("Failed to get nodes from the API server: %s", err.Error())
		return stats, err
	}

	for _, node := range nodes {
		if node.Metadata == nil || node.Metadata.Name == nil {
			log.Error("Incorrect payload when evaluating a node for the service mapper") // This will be removed as we move to the client-go
			continue
		}
		nodePodMetadataMap[*node.Metadata.Name], err = getMetadataMapBundle(*node.Metadata.Name)
		if err != nil {
			warn = fmt.Sprintf("Node %s could not be added to the service map bundle: %s", *node.Metadata.Name, err.Error())
			warnlist = append(warnlist, warn)
		}
	}
	stats["Nodes"] = nodePodMetadataMap
	stats["Warnings"] = warnlist
	return stats, nil
}

// GetMetadataMapBundleOnNode is used for the CLI metamap command to output given a nodeName.
func GetMetadataMapBundleOnNode(nodeName string) (map[string]interface{}, error) {
	nodePodMetadataMap := make(map[string]*MetadataMapperBundle)
	stats := make(map[string]interface{})
	var err error

	nodePodMetadataMap[nodeName], err = getMetadataMapBundle(nodeName)
	if err != nil {
		stats["Warnings"] = []string{fmt.Sprintf("Node %s could not be added to the metadata map bundle: %s", nodeName, err.Error())}
		return stats, err
	}
	stats["Nodes"] = nodePodMetadataMap
	return stats, nil
}

func getMetadataMapBundle(nodeName string) (*MetadataMapperBundle, error) {
	nodeNameCacheKey := cache.BuildAgentKey(metadataMapperCachePrefix, nodeName)
	metaBundle, found := cache.Cache.Get(nodeNameCacheKey)
	if !found {
		return nil, fmt.Errorf("the key %s was not found in the cache", nodeNameCacheKey)
	}
	return metaBundle.(*MetadataMapperBundle), nil
}

func getNodeList() ([]*v1.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5) // In case there are thousands of nodes.
	defer cancel()
	cl, err := GetAPIClient()
	if err != nil {
		log.Errorf("Can't create client to query the API Server: %s", err.Error())
		return nil, err
	}
	nodes, err := cl.client.CoreV1().ListNodes(ctx)
	if err != nil {
		log.Errorf("Can't list nodes from the API server: %s", err.Error())
		return nil, err
	}
	return nodes.Items, nil
}
