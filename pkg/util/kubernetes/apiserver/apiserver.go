// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	wpa_client "github.com/DataDog/watermarkpodautoscaler/pkg/client/clientset/versioned"
	wpa_informers "github.com/DataDog/watermarkpodautoscaler/pkg/client/informers/externalversions"

	dd_client "github.com/DataDog/datadog-operator/pkg/generated/clientset/versioned"
	dd_informers "github.com/DataDog/datadog-operator/pkg/generated/informers/externalversions"
)

var (
	globalAPIClient  *APIClient
	ErrNotFound      = errors.New("entity not found")
	ErrIsEmpty       = errors.New("entity is empty")
	ErrNotLeader     = errors.New("not Leader")
	isConnectVerbose = false
)

const (
	configMapDCAToken         = "datadogtoken"
	tokenTime                 = "tokenTimestamp"
	tokenKey                  = "tokenKey"
	metadataMapExpire         = 2 * time.Minute
	metadataMapperCachePrefix = "KubernetesMetadataMapping"
)

// APIClient provides authenticated access to the
// apiserver endpoints. Use the shared instance via GetApiClient.
type APIClient struct {
	// InformerFactory gives access to informers.
	InformerFactory informers.SharedInformerFactory

	// UnassignedPodInformerFactory gives access to filtered informers
	UnassignedPodInformerFactory informers.SharedInformerFactory

	// CertificateSecretInformerFactory gives access to filtered informers
	// This informer can be used by the Admission Controller to only watch the secret object
	// that contains the webhook certificate.
	CertificateSecretInformerFactory informers.SharedInformerFactory

	// WebhookConfigInformerFactory gives access to filtered informers
	// This informer can be used by the Admission Controller to only watch
	// the corresponding MutatingWebhookConfiguration object.
	WebhookConfigInformerFactory informers.SharedInformerFactory

	// WPAClient gives access to WPA API
	WPAClient wpa_client.Interface
	// WPAInformerFactory gives access to informers for Watermark Pod Autoscalers.
	WPAInformerFactory wpa_informers.SharedInformerFactory

	// DDClient gives access to all datadoghq/ custom types
	DDClient dd_client.Interface
	// DDInformerFactory gives access to informers for all datadoghq/ custom types
	DDInformerFactory dd_informers.SharedInformerFactory

	// used to setup the APIClient
	initRetry      retry.Retrier
	Cl             kubernetes.Interface
	DynamicCl      dynamic.Interface
	timeoutSeconds int64
}

// GetAPIClient returns the shared ApiClient instance.
func GetAPIClient() (*APIClient, error) {
	if globalAPIClient == nil {
		globalAPIClient = &APIClient{
			timeoutSeconds: config.Datadog.GetInt64("kubernetes_apiserver_client_timeout"),
		}
		globalAPIClient.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "apiserver",
			AttemptMethod:     globalAPIClient.connect,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	err := globalAPIClient.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("API Server init error: %s", err)
		return nil, err
	}
	return globalAPIClient, nil
}

func getClientConfig() (*rest.Config, error) {
	var clientConfig *rest.Config
	var err error
	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		clientConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Debugf("Can't create a config for the official client from the service account's token: %v", err)
			return nil, err
		}
	} else {
		// use the current context in kubeconfig
		clientConfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			log.Debugf("Can't create a config for the official client from the configured path to the kubeconfig: %s, %v", cfgPath, err)
			return nil, err
		}
	}

	if config.Datadog.GetBool("kubernetes_apiserver_use_protobuf") {
		clientConfig.ContentType = "application/vnd.kubernetes.protobuf"
	}
	return clientConfig, nil
}

func getKubeClient(timeout time.Duration) (kubernetes.Interface, error) {
	clientConfig, err := getClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = timeout
	return kubernetes.NewForConfig(clientConfig)
}

func getDynamicKubeClient(timeout time.Duration) (dynamic.Interface, error) {
	clientConfig, err := getClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = timeout
	return dynamic.NewForConfig(clientConfig)
}

func getWPAClient(timeout time.Duration) (wpa_client.Interface, error) {
	clientConfig, err := getClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = timeout
	return wpa_client.NewForConfig(clientConfig)
}

func getWPAInformerFactory() (wpa_informers.SharedInformerFactory, error) {
	// default to 300s
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getWPAClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return nil, err
	}
	return wpa_informers.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getDDClient(timeout time.Duration) (dd_client.Interface, error) {
	clientConfig, err := getClientConfig()
	if err != nil {
		return nil, err
	}
	clientConfig.Timeout = timeout
	return dd_client.NewForConfig(clientConfig)
}

func getDDInformerFactory() (dd_informers.SharedInformerFactory, error) {
	// default to 300s
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getDDClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return nil, err
	}
	return dd_informers.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getInformerFactory() (informers.SharedInformerFactory, error) {
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getKubeClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Errorf("Could not get apiserver client: %v", err)
		return nil, err
	}
	return informers.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getInformerFactoryWithOption(options informers.SharedInformerOption) (informers.SharedInformerFactory, error) {
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getKubeClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Errorf("Could not get apiserver client: %v", err)
		return nil, err
	}
	return informers.NewSharedInformerFactoryWithOptions(client, resyncPeriodSeconds*time.Second, options), nil
}

func (c *APIClient) connect() error {
	var err error
	c.Cl, err = getKubeClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return err
	}

	// informer factory uses its own clientset with a larger timeout
	c.InformerFactory, err = getInformerFactory()
	if err != nil {
		return err
	}

	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		tweakListOptions := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", "").String()
		}
		c.UnassignedPodInformerFactory, err = getInformerFactoryWithOption(
			informers.WithTweakListOptions(tweakListOptions),
		)
	}

	if config.Datadog.GetBool("admission_controller.enabled") {
		nameFieldkey := "metadata.name"
		optionsForService := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.certificate.secret_name")).String()
		}
		c.CertificateSecretInformerFactory, err = getInformerFactoryWithOption(
			informers.WithTweakListOptions(optionsForService),
		)

		optionsForWebhook := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.webhook_name")).String()
		}
		c.WebhookConfigInformerFactory, err = getInformerFactoryWithOption(
			informers.WithTweakListOptions(optionsForWebhook),
		)

		c.DynamicCl, err = getDynamicKubeClient(time.Duration(c.timeoutSeconds) * time.Second)
		if err != nil {
			log.Infof("Could not get apiserver dynamic client: %v", err)
			return err
		}
	}

	if config.Datadog.GetBool("external_metrics_provider.wpa_controller") {
		if c.WPAInformerFactory, err = getWPAInformerFactory(); err != nil {
			log.Errorf("Error getting WPA Informer Factory: %s", err.Error())
			return err
		}
		if c.WPAClient, err = getWPAClient(time.Duration(c.timeoutSeconds) * time.Second); err != nil {
			log.Errorf("Error getting WPA Client: %s", err.Error())
			return err
		}
	}
	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		if c.DDInformerFactory, err = getDDInformerFactory(); err != nil {
			log.Errorf("Error getting datadoghq Client: %s", err.Error())
			return err
		}
		if c.DDClient, err = getDDClient(time.Duration(c.timeoutSeconds) * time.Second); err != nil {
			log.Errorf("Error getting datadoghq Informer Factory: %s", err.Error())
			return err
		}
	}
	// Try to get apiserver version to confim connectivity
	APIversion := c.Cl.Discovery().RESTClient().APIVersion()
	if APIversion.Empty() {
		return fmt.Errorf("cannot retrieve the version of the API server at the moment")
	}
	log.Debugf("Connected to kubernetes apiserver, version %s", APIversion.Version)

	err = c.checkResourcesAuth()
	if err != nil {
		return err
	}
	log.Debug("Could successfully collect Pods, Nodes, Services and Events")
	return nil
}

// metadataMapperBundle maps pod names to associated metadata.
type metadataMapperBundle struct {
	Services apiv1.NamespacesPodsStringsSet
	mapOnIP  bool // temporary opt-out of the new mapping logic
}

func newMetadataMapperBundle() *metadataMapperBundle {
	return &metadataMapperBundle{
		Services: apiv1.NewNamespacesPodsStringsSet(),
		mapOnIP:  config.Datadog.GetBool("kubernetes_map_services_on_ip"),
	}
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
	var errorMessages []string

	// We always want to collect events
	_, err := c.Cl.CoreV1().Events("").List(metav1.ListOptions{Limit: 1, TimeoutSeconds: &c.timeoutSeconds})
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("event collection: %q", err.Error()))
		if !isConnectVerbose {
			return aggregateCheckResourcesErrors(errorMessages)
		}
	}

	if config.Datadog.GetBool("kubernetes_collect_metadata_tags") == false {
		return aggregateCheckResourcesErrors(errorMessages)
	}

	_, err = c.Cl.CoreV1().Services("").List(metav1.ListOptions{Limit: 1, TimeoutSeconds: &c.timeoutSeconds})
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("service collection: %q", err.Error()))
		if !isConnectVerbose {
			return aggregateCheckResourcesErrors(errorMessages)
		}
	}

	_, err = c.Cl.CoreV1().Pods("").List(metav1.ListOptions{Limit: 1, TimeoutSeconds: &c.timeoutSeconds})
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("pod collection: %q", err.Error()))
		if !isConnectVerbose {
			return aggregateCheckResourcesErrors(errorMessages)
		}
	}

	_, err = c.Cl.CoreV1().Nodes().List(metav1.ListOptions{Limit: 1, TimeoutSeconds: &c.timeoutSeconds})
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("node collection: %q", err.Error()))
	}

	if c.DDClient != nil {
		_, err = c.DDClient.DatadoghqV1alpha1().DatadogMetrics("").List(metav1.ListOptions{Limit: 1, TimeoutSeconds: &c.timeoutSeconds})
		if err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("DatadogMetric collection: %q", err.Error()))
		}
	}

	return aggregateCheckResourcesErrors(errorMessages)
}

// ComponentStatuses returns the component status list from the APIServer
func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error) {
	return c.Cl.CoreV1().ComponentStatuses().List(metav1.ListOptions{TimeoutSeconds: &c.timeoutSeconds})
}

func (c *APIClient) getOrCreateConfigMap(name, namespace string) (cmEvent *v1.ConfigMap, err error) {
	cmEvent, err = c.Cl.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not get the ConfigMap %s: %s, trying to create it.", name, err.Error())
		cmEvent, err = c.Cl.CoreV1().ConfigMaps(namespace).Create(&v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("could not create the ConfigMap: %s", err.Error())
		}
		log.Infof("Created the ConfigMap %s", name)
	}
	return cmEvent, nil
}

// GetTokenFromConfigmap returns the value of the `tokenValue` from the `tokenKey` in the ConfigMap `configMapDCAToken` if its timestamp is less than tokenTimeout old.
func (c *APIClient) GetTokenFromConfigmap(token string) (string, time.Time, error) {
	namespace := common.GetResourcesNamespace()
	nowTs := time.Now()

	cmEvent, err := c.getOrCreateConfigMap(configMapDCAToken, namespace)
	if err != nil {
		// we do not process event if we can't interact with the CM.
		return "", time.Now(), err
	}
	eventTokenKey := fmt.Sprintf("%s.%s", token, tokenKey)
	if cmEvent.Data == nil {
		cmEvent.Data = make(map[string]string)
	}
	tokenValue, found := cmEvent.Data[eventTokenKey]
	if !found {
		log.Debugf("%s was not found in the ConfigMap %s, updating it to resync.", eventTokenKey, configMapDCAToken)
		// we should try to set it to "" .
		err = c.UpdateTokenInConfigmap(token, "", time.Now())
		return "", time.Now(), err
	}
	log.Tracef("%s is %q", token, tokenValue)

	eventTokenTS := fmt.Sprintf("%s.%s", token, tokenTime)
	tokenTimeStr, set := cmEvent.Data[eventTokenTS]
	if !set {
		log.Debugf("Could not find timestamp associated with %s in the ConfigMap %s. Refreshing.", eventTokenTS, configMapDCAToken)
		// The timestamp of the last List is not present, it will be set during the next Collection.
		return tokenValue, nowTs, nil
	}

	tokenTime, err := time.Parse(time.RFC3339, tokenTimeStr)
	if err != nil {
		log.Errorf("Could not convert the timestamp associated with %s from the ConfigMap %s, resync might not work correctly.", token, configMapDCAToken)
		return tokenValue, nowTs, nil
	}
	return tokenValue, tokenTime, err
}

// UpdateTokenInConfigmap updates the value of the `tokenValue` from the `tokenKey` and
// sets its collected timestamp in the ConfigMap `configmaptokendca`
func (c *APIClient) UpdateTokenInConfigmap(token, tokenValue string, timestamp time.Time) error {
	namespace := common.GetResourcesNamespace()
	tokenConfigMap, err := c.getOrCreateConfigMap(configMapDCAToken, namespace)
	if err != nil {
		return err
	}
	eventTokenKey := fmt.Sprintf("%s.%s", token, tokenKey)
	if tokenConfigMap.Data == nil {
		tokenConfigMap.Data = make(map[string]string)
	}
	tokenConfigMap.Data[eventTokenKey] = tokenValue

	eventTokenTS := fmt.Sprintf("%s.%s", token, tokenTime)
	tokenConfigMap.Data[eventTokenTS] = timestamp.Format(time.RFC3339) // Timestamps in the ConfigMap should all use the type int.

	_, err = c.Cl.CoreV1().ConfigMaps(namespace).Update(tokenConfigMap)
	if err != nil {
		return err
	}
	log.Debugf("Updated %s to %s in the ConfigMap %s", eventTokenKey, tokenValue, configMapDCAToken)
	return nil
}

// NodeLabels is used to fetch the labels attached to a given node.
func (c *APIClient) NodeLabels(nodeName string) (map[string]string, error) {
	node, err := c.Cl.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node.Labels, nil
}

// GetNodeForPod retrieves a pod and returns the name of the node it is scheduled on
func (c *APIClient) GetNodeForPod(namespace, podName string) (string, error) {
	pod, err := c.Cl.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Spec.NodeName, nil
}

// GetMetadataMapBundleOnAllNodes is used for the CLI svcmap command to run fetch the metadata map of all nodes.
func GetMetadataMapBundleOnAllNodes(cl *APIClient) (*apiv1.MetadataResponse, error) {
	stats := apiv1.NewMetadataResponse()
	var err error

	nodes, err := getNodeList(cl)
	if err != nil {
		stats.Errors = fmt.Sprintf("Failed to get nodes from the API server: %s", err.Error())
		return stats, err
	}

	for _, node := range nodes {
		if node.GetObjectMeta() == nil {
			log.Error("Incorrect payload when evaluating a node for the service mapper") // This will be removed as we move to the client-go
			continue
		}
		var bundle *metadataMapperBundle
		bundle, err = getMetadataMapBundle(node.Name)
		if err != nil {
			warn := fmt.Sprintf("Node %s could not be added to the service map bundle: %s", node.Name, err.Error())
			stats.Warnings = append(stats.Warnings, warn)
			continue
		}
		stats.Nodes[node.Name] = convertmetadataMapperBundleToAPI(bundle)
	}
	return stats, nil
}

// GetMetadataMapBundleOnNode is used for the CLI metamap command to output given a nodeName.
func GetMetadataMapBundleOnNode(nodeName string) (*apiv1.MetadataResponse, error) {
	stats := apiv1.NewMetadataResponse()
	bundle, err := getMetadataMapBundle(nodeName)
	if err != nil {
		stats.Warnings = []string{fmt.Sprintf("Node %s could not be added to the metadata map bundle: %s", nodeName, err.Error())}
		return stats, err
	}

	stats.Nodes[nodeName] = convertmetadataMapperBundleToAPI(bundle)
	return stats, nil
}

func getMetadataMapBundle(nodeName string) (*metadataMapperBundle, error) {
	nodeNameCacheKey := cache.BuildAgentKey(metadataMapperCachePrefix, nodeName)
	metaBundle, found := cache.Cache.Get(nodeNameCacheKey)
	if !found {
		return nil, fmt.Errorf("the key %s was not found in the cache", nodeNameCacheKey)
	}
	return metaBundle.(*metadataMapperBundle), nil
}

func getNodeList(cl *APIClient) ([]v1.Node, error) {
	nodes, err := cl.Cl.CoreV1().Nodes().List(metav1.ListOptions{TimeoutSeconds: &cl.timeoutSeconds})
	if err != nil {
		log.Errorf("Can't list nodes from the API server: %s", err.Error())
		return nil, err
	}
	return nodes.Items, nil
}

// GetNode retrieves a node by name
func GetNode(cl *APIClient, name string) (*v1.Node, error) {
	node, err := cl.Cl.CoreV1().Nodes().Get(name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Can't get node from the API server: %s", err.Error())
		return nil, err
	}
	return node, nil
}

// GetRESTObject allows to retrieve a custom resource from the APIserver
func (c *APIClient) GetRESTObject(path string, output runtime.Object) error {
	result := c.Cl.CoreV1().RESTClient().Get().AbsPath(path).Do()
	if result.Error() != nil {
		return result.Error()
	}

	return result.Into(output)
}

func convertmetadataMapperBundleToAPI(input *metadataMapperBundle) *apiv1.MetadataResponseBundle {
	output := apiv1.NewMetadataResponseBundle()
	if input == nil {
		return output
	}
	for key, val := range input.Services {
		output.Services[key] = val
	}
	return output
}
