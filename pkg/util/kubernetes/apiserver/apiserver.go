// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package apiserver provides an API client for the Kubernetes API server.
package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	apiregistrationclient "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"
)

var (
	globalAPIClient     *APIClient
	globalAPIClientOnce sync.Once
	ErrNotFound         = errors.New("entity not found") //nolint:revive
	ErrIsEmpty          = errors.New("entity is empty")  //nolint:revive
	ErrNotLeader        = errors.New("not Leader")       //nolint:revive
)

const (
	tokenTime                 = "tokenTimestamp"
	tokenKey                  = "tokenKey"
	metadataMapExpire         = 2 * time.Minute
	metadataMapperCachePrefix = "KubernetesMetadataMapping"
)

// APIClient provides authenticated access to the
// apiserver endpoints. Use the shared instance via GetApiClient.
type APIClient struct {
	//
	// Normal (low timeout) clients
	//

	// Cl holds the main kubernetes client
	Cl kubernetes.Interface

	// DynamicCl holds a dynamic kubernetes client
	DynamicCl dynamic.Interface

	//
	// Informer clients (high or no timeout, use for Informers/Watch calls)
	//

	// InformerCl holds the main kubernetes client with long TO
	InformerCl kubernetes.Interface

	// DynamicCl holds a dynamic kubernetes client with long TO
	DynamicInformerCl dynamic.Interface

	// CRDInformerClient holds the extension kubernetes client with long TO
	CRDInformerClient clientset.Interface

	// APISInformerClient holds the APIService kubernetes client with long TO
	APISInformerClient apiregistrationclient.ApiregistrationV1Interface

	// VPAInformerClient holds kubernetes VerticalPodAutoscalers client with long TO
	VPAInformerClient vpa.Interface

	//
	// Informer factories (based on informers client)
	// Factories are not started by default.
	// Factories can be started multiple times safely.
	// Factories CANNOT be stopped safely. It means that you can only use these informers
	// if informer lifetime == agent lifetime (e.g. not in CLC and not if leader-based).
	//
	// Use the Informer* clients above to create your own factories if you need to.
	//

	// InformerFactory gives access to informers.
	InformerFactory informers.SharedInformerFactory

	// CertificateSecretInformerFactory gives access to filtered informers
	// This informer can be used by the Admission Controller to only watch the secret object
	// that contains the webhook certificate.
	CertificateSecretInformerFactory informers.SharedInformerFactory

	// WebhookConfigInformerFactory gives access to filtered informers
	// This informer can be used by the Admission Controller to only watch
	// the corresponding MutatingWebhookConfiguration object.
	WebhookConfigInformerFactory informers.SharedInformerFactory

	// DynamicInformerFactory gives access to dynamic informers
	DynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory

	//
	// Internal
	//

	// initRetry used to setup the APIClient
	initRetry retry.Retrier

	// Client and informer timeout
	defaultClientTimeout        time.Duration
	defaultInformerTimeout      time.Duration
	defaultInformerResyncPeriod time.Duration
}

func initAPIClient() {
	globalAPIClient = &APIClient{
		defaultClientTimeout:        time.Duration(config.Datadog.GetInt64("kubernetes_apiserver_client_timeout")) * time.Second,
		defaultInformerTimeout:      time.Duration(config.Datadog.GetInt64("kubernetes_apiserver_informer_client_timeout")) * time.Second,
		defaultInformerResyncPeriod: time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period")) * time.Second,
	}
	globalAPIClient.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
		Name:              "apiserver",
		AttemptMethod:     globalAPIClient.connect,
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     5 * time.Minute,
	})
}

// GetAPIClient returns the shared APIClient if already set
// it will trigger a retry if not, but won't wait until retries are exhausted
// See `WaitForAPIClient()` for a method that waits until APIClient is ready
func GetAPIClient() (*APIClient, error) {
	globalAPIClientOnce.Do(initAPIClient)

	err := globalAPIClient.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("API Server init error: %s", err)
		return nil, err
	}
	return globalAPIClient, nil
}

// WaitForAPIClient waits for availability of APIServer Client before returning
func WaitForAPIClient(ctx context.Context) (*APIClient, error) {
	globalAPIClientOnce.Do(initAPIClient)

	for {
		_ = globalAPIClient.initRetry.TriggerRetry()
		switch globalAPIClient.initRetry.RetryStatus() {
		case retry.OK:
			return globalAPIClient, nil
		case retry.PermaFail:
			return nil, fmt.Errorf("Permanent failure while waiting for Kubernetes APIServer")
		default:
			sleepFor := globalAPIClient.initRetry.NextRetry().UTC().Sub(time.Now().UTC()) + time.Second
			log.Debugf("Waiting for APIServer, next retry: %v", sleepFor)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("Context deadline reached while waiting for Kubernetes APIServer")
			case <-time.After(sleepFor):
			}
		}
	}
}

func GetClientConfig(timeout time.Duration) (*rest.Config, error) {
	var clientConfig *rest.Config
	var err error
	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		clientConfig, err = rest.InClusterConfig()

		if !config.Datadog.GetBool("kubernetes_apiserver_tls_verify") {
			clientConfig.TLSClientConfig.Insecure = true
		}

		if customCAPath := config.Datadog.GetString("kubernetes_apiserver_ca_path"); customCAPath != "" {
			clientConfig.TLSClientConfig.CAFile = customCAPath
		}

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

	clientConfig.Timeout = timeout
	clientConfig.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return NewCustomRoundTripper(rt, timeout)
	})

	return clientConfig, nil
}

// GetKubeClient returns a kubernetes API server client
// You should not use this function except if you need to create a *NEW* Client.
func GetKubeClient(timeout time.Duration) (kubernetes.Interface, error) {
	// TODO: Remove custom warning logger when we remove usage of ComponentStatus
	rest.SetDefaultWarningHandler(CustomWarningLogger{})
	clientConfig, err := GetClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(clientConfig)
}

func getKubeDynamicClient(timeout time.Duration) (dynamic.Interface, error) {
	clientConfig, err := GetClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(clientConfig)
}

func getCRDClient(timeout time.Duration) (*clientset.Clientset, error) {
	clientConfig, err := GetClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return clientset.NewForConfig(clientConfig)
}

func getAPISClient(timeout time.Duration) (*apiregistrationclient.ApiregistrationV1Client, error) {
	clientConfig, err := GetClientConfig(timeout)
	if err != nil {
		return nil, err
	}
	return apiregistrationclient.NewForConfig(clientConfig)
}

func getKubeVPAClient(timeout time.Duration) (vpa.Interface, error) {
	clientConfig, err := GetClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return vpa.NewForConfig(clientConfig)
}

// GetInformerWithOptions returns
func (c *APIClient) GetInformerWithOptions(resyncPeriod *time.Duration, options ...informers.SharedInformerOption) informers.SharedInformerFactory {
	if resyncPeriod == nil {
		resyncPeriod = &c.defaultInformerResyncPeriod
	}

	return informers.NewSharedInformerFactoryWithOptions(c.InformerCl, *resyncPeriod, options...)
}

func (c *APIClient) connect() error {
	var err error

	// Clients
	c.Cl, err = GetKubeClient(c.defaultClientTimeout)
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return err
	}

	c.DynamicCl, err = getKubeDynamicClient(c.defaultClientTimeout)
	if err != nil {
		log.Infof("Could not get apiserver dynamic client: %v", err)
		return err
	}

	// Informer clients
	c.InformerCl, err = GetKubeClient(c.defaultInformerTimeout)
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return err
	}

	c.DynamicInformerCl, err = getKubeDynamicClient(c.defaultInformerTimeout)
	if err != nil {
		log.Infof("Could not get apiserver dynamic client: %v", err)
		return err
	}

	c.VPAInformerClient, err = getKubeVPAClient(c.defaultInformerTimeout)
	if err != nil {
		log.Infof("Could not get apiserver vpa client: %v", err)
		return err
	}

	c.CRDInformerClient, err = getCRDClient(c.defaultInformerTimeout)
	if err != nil {
		log.Infof("Could not get apiserver CRDClient client: %v", err)
		return err
	}

	c.APISInformerClient, err = getAPISClient(c.defaultInformerTimeout)
	if err != nil {
		log.Infof("Could not get apiserver APISClient client: %v", err)
		return err
	}

	// Creating informers
	c.InformerFactory = c.GetInformerWithOptions(nil)

	if config.Datadog.GetBool("admission_controller.enabled") ||
		config.Datadog.GetBool("compliance_config.enabled") ||
		config.Datadog.GetBool("orchestrator_explorer.enabled") ||
		config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") ||
		config.Datadog.GetBool("external_metrics_provider.wpa_controller") ||
		config.Datadog.GetBool("cluster_checks.enabled") {
		c.DynamicInformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(c.DynamicInformerCl, c.defaultInformerResyncPeriod)
	}

	if config.Datadog.GetBool("admission_controller.enabled") {
		nameFieldkey := "metadata.name"
		optionsForService := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.certificate.secret_name")).String()
		}
		c.CertificateSecretInformerFactory = c.GetInformerWithOptions(
			nil,
			informers.WithTweakListOptions(optionsForService),
			informers.WithNamespace(common.GetResourcesNamespace()),
		)

		optionsForWebhook := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.webhook_name")).String()
		}
		c.WebhookConfigInformerFactory = c.GetInformerWithOptions(
			nil,
			informers.WithTweakListOptions(optionsForWebhook),
		)
	}

	// Try to get apiserver version to confim connectivity
	APIversion := c.Cl.Discovery().RESTClient().APIVersion()
	if APIversion.Empty() {
		return fmt.Errorf("cannot retrieve the version of the API server at the moment")
	}
	log.Debugf("Connected to kubernetes apiserver, version %s", APIversion.Version)

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

// ComponentStatuses returns the component status list from the APIServer
func (c *APIClient) ComponentStatuses() (*v1.ComponentStatusList, error) {
	return c.Cl.CoreV1().ComponentStatuses().List(context.TODO(), metav1.ListOptions{TimeoutSeconds: pointer.Ptr(int64(c.defaultClientTimeout.Seconds()))})
}

func (c *APIClient) getOrCreateConfigMap(name, namespace string) (cmEvent *v1.ConfigMap, err error) {
	cmEvent, err = c.Cl.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Could not get the ConfigMap %s: %s, trying to create it.", name, err.Error())
		cmEvent, err = c.Cl.CoreV1().ConfigMaps(namespace).Create(context.TODO(), &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}, metav1.CreateOptions{})
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

	configMapDCAToken := config.Datadog.GetString("cluster_agent.token_name")
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
	configMapDCAToken := config.Datadog.GetString("cluster_agent.token_name")
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

	_, err = c.Cl.CoreV1().ConfigMaps(namespace).Update(context.TODO(), tokenConfigMap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	log.Debugf("Updated %s to %s in the ConfigMap %s", eventTokenKey, tokenValue, configMapDCAToken)
	return nil
}

// NodeLabels is used to fetch the labels attached to a given node.
func (c *APIClient) NodeLabels(nodeName string) (map[string]string, error) {
	node, err := c.Cl.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node.Labels, nil
}

// NodeAnnotations is used to fetch the annotations attached to a given node.
func (c *APIClient) NodeAnnotations(nodeName string) (map[string]string, error) {
	node, err := c.Cl.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node.Annotations, nil
}

// GetNodeForPod retrieves a pod and returns the name of the node it is scheduled on
func (c *APIClient) GetNodeForPod(ctx context.Context, namespace, podName string) (string, error) {
	pod, err := c.Cl.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
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
	nodes, err := cl.Cl.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{TimeoutSeconds: pointer.Ptr(int64(cl.defaultClientTimeout.Seconds()))})
	if err != nil {
		log.Errorf("Can't list nodes from the API server: %s", err.Error())
		return nil, err
	}
	return nodes.Items, nil
}

// GetNode retrieves a node by name
func GetNode(cl *APIClient, name string) (*v1.Node, error) {
	node, err := cl.Cl.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Can't get node from the API server: %s", err.Error())
		return nil, err
	}
	return node, nil
}

// GetRESTObject allows to retrieve a custom resource from the APIserver
func (c *APIClient) GetRESTObject(path string, output runtime.Object) error {
	result := c.Cl.CoreV1().RESTClient().Get().AbsPath(path).Do(context.TODO())
	if result.Error() != nil {
		return result.Error()
	}

	return result.Into(output)
}

// IsAPIServerReady retrieves the API Server readiness status
func (c *APIClient) IsAPIServerReady(ctx context.Context) (bool, error) {
	path := "/readyz"
	_, err := c.Cl.Discovery().RESTClient().Get().AbsPath(path).DoRaw(ctx)

	return err == nil, err
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

// GetARandomNodeName chooses a random node and returns its name. Returns an
// error if there are no nodes or if there's an error listing them.
func (c *APIClient) GetARandomNodeName(ctx context.Context) (string, error) {
	nodeList, err := c.Cl.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		return "", err
	}

	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("No node found")
	}

	return nodeList.Items[0].Name, nil
}
