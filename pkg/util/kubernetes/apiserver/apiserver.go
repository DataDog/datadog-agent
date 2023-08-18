// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

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
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	vpai "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/discovery"
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
	WPAClient dynamic.Interface
	// WPAInformerFactory gives access to informers for Watermark Pod Autoscalers.
	WPAInformerFactory dynamicinformer.DynamicSharedInformerFactory

	// DDClient gives access to all datadoghq/ custom types
	DDClient dynamic.Interface
	// DynamicInformerFactory gives access to dynamic informers in example for all datadoghq/ custom types
	DynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory

	// CRDInformerFactory gives access to informers for all crds
	CRDInformerFactory externalversions.SharedInformerFactory

	// initRetry used to setup the APIClient
	initRetry retry.Retrier

	// Cl holds the main kubernetes client
	Cl kubernetes.Interface

	// CRDClient holds the extension kubernetes client
	CRDClient clientset.Interface

	// APISClient holds the APIService kubernetes client
	APISClient apiregistrationclient.ApiregistrationV1Interface

	// DynamicCl holds a dynamic kubernetes client
	DynamicCl dynamic.Interface

	// DiscoveryCl holds kubernetes discovery client
	DiscoveryCl discovery.DiscoveryInterface

	// VPAClient holds kubernetes VerticalPodAutoscalers client
	VPAClient vpa.Interface

	// VPAInformerFactory
	VPAInformerFactory vpai.SharedInformerFactory

	// timeoutSeconds defines the kubernetes client timeout
	timeoutSeconds int64
}

func initAPIClient() {
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

func getClientConfig(timeout time.Duration) (*rest.Config, error) {
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
		return NewCustomRoundTripper(rt)
	})

	return clientConfig, nil
}

// GetKubeClient returns a kubernetes API server client
func GetKubeClient(timeout time.Duration) (kubernetes.Interface, error) {
	// TODO: Remove custom warning logger when we remove usage of ComponentStatus
	rest.SetDefaultWarningHandler(CustomWarningLogger{})
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(clientConfig)
}

func getKubeDynamicClient(timeout time.Duration) (dynamic.Interface, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(clientConfig)
}

func getCRDClient(timeout time.Duration) (*clientset.Clientset, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return clientset.NewForConfig(clientConfig)
}

func getAPISClient(timeout time.Duration) (*apiregistrationclient.ApiregistrationV1Client, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}
	return apiregistrationclient.NewForConfig(clientConfig)
}

func getKubeDiscoveryClient(timeout time.Duration) (discovery.DiscoveryInterface, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return discovery.NewDiscoveryClientForConfig(clientConfig)
}

func getKubeVPAClient(timeout time.Duration) (vpa.Interface, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return vpa.NewForConfig(clientConfig)
}

// VPAInformerFactory vpai.SharedInformerFactory
func getVPAInformerFactory(client vpa.Interface) (vpai.SharedInformerFactory, error) {
	// default to 300s
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	return vpai.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getWPAInformerFactory() (dynamicinformer.DynamicSharedInformerFactory, error) {
	// default to 300s
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getKubeDynamicClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return nil, err
	}
	return dynamicinformer.NewDynamicSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getDDClient(timeout time.Duration) (dynamic.Interface, error) {
	clientConfig, err := getClientConfig(timeout)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(clientConfig)
}

func getDDInformerFactory() (dynamicinformer.DynamicSharedInformerFactory, error) {
	// default to 300s
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getKubeDynamicClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return nil, err
	}
	return dynamicinformer.NewDynamicSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getInformerFactory() (informers.SharedInformerFactory, error) {
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := GetKubeClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Errorf("Could not get apiserver client: %v", err)
		return nil, err
	}
	return informers.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getCRDInformerFactory() (externalversions.SharedInformerFactory, error) {
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := getCRDClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Errorf("Could not get apiserver client: %v", err)
		return nil, err
	}
	return externalversions.NewSharedInformerFactory(client, resyncPeriodSeconds*time.Second), nil
}

func getInformerFactoryWithOption(options ...informers.SharedInformerOption) (informers.SharedInformerFactory, error) {
	resyncPeriodSeconds := time.Duration(config.Datadog.GetInt64("kubernetes_informers_resync_period"))
	client, err := GetKubeClient(0) // No timeout for the Informers, to allow long watch.
	if err != nil {
		log.Errorf("Could not get apiserver client: %v", err)
		return nil, err
	}
	return informers.NewSharedInformerFactoryWithOptions(client, resyncPeriodSeconds*time.Second, options...), nil
}

func (c *APIClient) connect() error {
	var err error
	c.Cl, err = GetKubeClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver client: %v", err)
		return err
	}

	c.DiscoveryCl, err = getKubeDiscoveryClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver discovery client: %v", err)
		return err
	}

	c.VPAClient, err = getKubeVPAClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver vpa client: %v", err)
		return err
	}

	c.CRDClient, err = getCRDClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver CRDClient client: %v", err)
		return err
	}

	c.APISClient, err = getAPISClient(time.Duration(c.timeoutSeconds) * time.Second)
	if err != nil {
		log.Infof("Could not get apiserver APISClient client: %v", err)
		return err
	}

	if config.Datadog.GetBool("admission_controller.enabled") ||
		config.Datadog.GetBool("compliance_config.enabled") ||
		config.Datadog.GetBool("orchestrator_explorer.enabled") ||
		config.Datadog.GetBool("cluster_checks.enabled") {
		c.DynamicCl, err = getKubeDynamicClient(time.Duration(c.timeoutSeconds) * time.Second)
		if err != nil {
			log.Infof("Could not get apiserver dynamic client: %v", err)
			return err
		}
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
		if err != nil {
			log.Infof("Could not get informer factory: %v", err)
			return err
		}
		if c.CRDInformerFactory, err = getCRDInformerFactory(); err != nil {
			_ = log.Errorf("Error getting crd informer Client: %s", err.Error())
			return err
		}
		if c.DynamicInformerFactory, err = getDDInformerFactory(); err != nil {
			_ = log.Errorf("Error getting datadoghq informer Client: %s", err.Error())
			return err
		}

		c.VPAInformerFactory, err = getVPAInformerFactory(c.VPAClient)
		if err != nil {
			log.Infof("Could not get a vpa informer factory: %v", err)
			return err
		}

	}

	if config.Datadog.GetBool("admission_controller.enabled") {
		nameFieldkey := "metadata.name"
		optionsForService := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.certificate.secret_name")).String()
		}
		c.CertificateSecretInformerFactory, err = getInformerFactoryWithOption(
			informers.WithTweakListOptions(optionsForService),
			informers.WithNamespace(common.GetResourcesNamespace()),
		)
		if err != nil {
			log.Infof("Could not get informer factory: %v", err)
			return err
		}

		optionsForWebhook := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector(nameFieldkey, config.Datadog.GetString("admission_controller.webhook_name")).String()
		}
		c.WebhookConfigInformerFactory, err = getInformerFactoryWithOption(
			informers.WithTweakListOptions(optionsForWebhook),
		)
		if err != nil {
			log.Infof("Could not get informer factory: %v", err)
			return err
		}

	}

	if config.Datadog.GetBool("external_metrics_provider.wpa_controller") {
		if c.WPAInformerFactory, err = getWPAInformerFactory(); err != nil {
			log.Errorf("Error getting WPA Informer Factory: %s", err.Error())
			return err
		}
		if c.WPAClient, err = getKubeDynamicClient(time.Duration(c.timeoutSeconds) * time.Second); err != nil {
			log.Errorf("Error getting WPA Client: %s", err.Error())
			return err
		}
	}
	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		if c.DynamicInformerFactory, err = getDDInformerFactory(); err != nil {
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
	return c.Cl.CoreV1().ComponentStatuses().List(context.TODO(), metav1.ListOptions{TimeoutSeconds: &c.timeoutSeconds})
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
	nodes, err := cl.Cl.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{TimeoutSeconds: &cl.timeoutSeconds})
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
