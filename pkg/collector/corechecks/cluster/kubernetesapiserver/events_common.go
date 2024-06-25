// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/patrickmn/go-cache"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	hostProviderIDCache *cache.Cache
)

type eventHostInfo struct {
	hostname   string
	nodename   string
	providerID string
}

// controllerToIntegration is a mapping of Kubernetes controller names to integrations
var controllerToIntegration = map[string]string{
	"targetgroupbinding":                             "amazon elb",
	"cilium-sidekick":                                "cilium",
	"datadogagent":                                   "datadog operator",
	"ExtendedDaemonSet":                              "datadog operator",
	"datadog-operator-manager":                       "datadog operator",
	"source-controller":                              "fluxcd",
	"kustomize-controller":                           "fluxcd",
	"karpenter":                                      "karpenter",
	"deployment-controller":                          "kubernetes",
	"job-controller":                                 "kubernetes",
	"node-controller":                                "kubernetes",
	"PodTemplate":                                    "kubernetes",
	"taint-controller":                               "kubernetes",
	"cronjob-controller":                             "kubernetes",
	"draino":                                         "kubernetes",
	"attachdetach-controller":                        "kubernetes",
	"horizontal-pod-autoscaler":                      "kubernetes",
	"daemonset-controller":                           "kubernetes",
	"cloud-node-controller":                          "kubernetes",
	"service-controller":                             "kubernetes",
	"taint-eviction-controller":                      "kubernetes",
	"cloud-controller-manager":                       "kubernetes",
	"wpa_controller":                                 "kubernetes",
	"workflow-controller":                            "kubernetes",
	"persistentvolume-controller":                    "kubernetes",
	"compute-diagnostics":                            "kubernetes",
	"disruption-budget-manager":                      "kubernetes",
	"vpa-updater":                                    "kubernetes",
	"serviceaccount-token-controller":                "kubernetes",
	"endpoints-controller":                           "kubernetes",
	"endpointslice-controller":                       "kubernetes",
	"endpointslice-mirroring-controller":             "kubernetes",
	"replicationcontroller-controller":               "kubernetes",
	"pod-garbage-collector-controller":               "kubernetes",
	"resourcequota-controller":                       "kubernetes",
	"namespace-controller":                           "kubernetes",
	"serviceaccount-controller":                      "kubernetes",
	"garbage-collector-controller":                   "kubernetes",
	"horizontal-pod-autoscaler-controller":           "kubernetes",
	"disruption-controller":                          "kubernetes",
	"statefulset-controller":                         "kubernetes",
	"certificatesigningrequest-signing-controller":   "kubernetes",
	"certificatesigningrequest-approving-controller": "kubernetes",
	"certificatesigningrequest-cleaner-controller":   "kubernetes",
	"ttl-controller":                                 "kubernetes",
	"bootstrap-signer-controller":                    "kubernetes",
	"token-cleaner-controller":                       "kubernetes",
	"node-ipam-controller":                           "kubernetes",
	"node-lifecycle-controller":                      "kubernetes",
	"persistentvolume-binder-controller":             "kubernetes",
	"persistentvolume-attach-detach-controller":      "kubernetes",
	"persistentvolume-expander-controller":           "kubernetes",
	"clusterrole-aggregation-controller":             "kubernetes",
	"persistentvolumeclaim-protection-controller":    "kubernetes",
	"persistentvolume-protection-controller":         "kubernetes",
	"ttl-after-finished-controller":                  "kubernetes",
	"storageversion-garbage-collector-controller":    "kubernetes",
	"resourceclaim-controller":                       "kubernetes",
	"legacy-serviceaccount-token-cleaner-controller": "kubernetes",
	"validatingadmissionpolicy-status-controller":    "kubernetes",
	"service-cidr-controller":                        "kubernetes",
	"storage-version-migrator-controller":            "kubernetes",
	"kubelet":                                        "kubernetes",
	"cluster-autoscaler":                             "kubernetes cluster autoscaler",
	"endpoint-controller":                            "kubernetes controller manager",
	"endpoint-slice-controller":                      "kubernetes controller manager",
	"replicaset-controller":                          "kubernetes controller manager",
	"kube-controller-manager":                        "kubernetes controller manager",
	"default-scheduler":                              "kube_scheduler",
	"spark-operator":                                 "spark",
	"vaultd":                                         "vault",
}

// defaultEventSource is the source that should be used for kubernetes events emitted by
// a controller not in the controllerToIntegration map.
const defaultEventSource = "kubernetes"

// kubernetesEventSource is the name of the source for kubernetes events
const kubernetesEventSource = "kubernetes"

// getDDAlertType converts kubernetes event types into datadog alert types
func getDDAlertType(k8sType string) event.AlertType {
	switch k8sType {
	case v1.EventTypeNormal:
		return event.AlertTypeInfo
	case v1.EventTypeWarning:
		return event.AlertTypeWarning
	default:
		log.Debugf("Unknown event type '%s'", k8sType)
		return event.AlertTypeInfo
	}
}

func getInvolvedObjectTags(involvedObject v1.ObjectReference, taggerInstance tagger.Component) []string {
	// NOTE: we now standardized on using kube_* tags, instead of
	// non-namespaced ones, or kubernetes_*. The latter two are now
	// considered deprecated.
	tagList := []string{
		fmt.Sprintf("kube_kind:%s", involvedObject.Kind),
		fmt.Sprintf("kube_name:%s", involvedObject.Name),

		// DEPRECATED:
		fmt.Sprintf("kubernetes_kind:%s", involvedObject.Kind),
		fmt.Sprintf("name:%s", involvedObject.Name),
	}

	if involvedObject.Namespace != "" {
		tagList = append(tagList,
			fmt.Sprintf("kube_namespace:%s", involvedObject.Namespace),

			// DEPRECATED:
			fmt.Sprintf("namespace:%s", involvedObject.Namespace),
		)

		namespaceEntityID := fmt.Sprintf("kubernetes_metadata://namespaces//%s", involvedObject.Namespace)
		namespaceEntity, err := taggerInstance.GetEntity(namespaceEntityID)
		if err == nil {
			tagList = append(tagList, namespaceEntity.GetTags(types.HighCardinality)...)
		}
	}

	kindTag := getKindTag(involvedObject.Kind, involvedObject.Name)
	if kindTag != "" {
		tagList = append(tagList, kindTag)
	}

	return tagList
}

const (
	podKind  = "Pod"
	nodeKind = "Node"
)

func getEventHostInfo(clusterName string, ev *v1.Event) eventHostInfo {
	return getEventHostInfoImpl(getHostProviderID, clusterName, ev)
}

// getEventHostInfoImpl get the host information (hostname,nodename) from where the event has been generated.
// This function takes `hostProviderIDFunc` function to ease unit-testing by mocking the
// providers logic
//
//nolint:revive // TODO(CINT) Fix revive linter
func getEventHostInfoImpl(hostProviderIDFunc func(string) string, clusterName string, ev *v1.Event) eventHostInfo {
	info := eventHostInfo{}

	switch ev.InvolvedObject.Kind {
	case podKind:
		info.nodename = ev.Source.Host
		// works fine with Pod's events generated by the kubelet, but not with other
		// source like the draino controller.
		// We should be able to resolve this issue with the workloadmetadatastore
		// in the cluster-agent
	case nodeKind:
		// on Node the host is not always provided in the ev.Source.Host
		// But it is always available in `ev.InvolvedObject.Name`
		info.nodename = ev.InvolvedObject.Name
	default:
		return info
	}

	info.hostname = info.nodename
	if info.hostname != "" {
		info.providerID = getHostProviderID(info.hostname)

		if clusterName != "" {
			info.hostname += "-" + clusterName
		}
	}

	return info
}

func getHostProviderID(nodename string) string {
	if hostProviderID, hit := hostProviderIDCache.Get(nodename); hit {
		return hostProviderID.(string)
	}

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		log.Warnf("Can't create client to query the API Server: %v", err)
		return ""
	}

	node, err := apiserver.GetNode(cl, nodename)
	if err != nil {
		log.Warnf("Can't get node from API Server: %v", err)
		return ""
	}

	providerID := node.Spec.ProviderID
	if providerID == "" {
		log.Warnf("ProviderID for nodename %q not found", nodename)
		return ""
	}

	// e.g. gce://datadog-test-cluster/us-east1-a/some-instance-id or
	// aws:///us-east-1e/i-instanceid
	s := strings.Split(providerID, "/")
	hostProviderID := s[len(s)-1]

	hostProviderIDCache.Set(nodename, hostProviderID, cache.DefaultExpiration)

	return hostProviderID
}

// getKindTag returns the kube_<kind>:<name> tag. The exact same tag names and
// object kinds are supported by the tagger. It returns an empty string if the
// kind doesn't correspond to a known/supported kind tag.
func getKindTag(kind, name string) string {
	tagName, err := tags.GetTagForKind(kind)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s:%s", tagName, name)
}

func buildReadableKey(obj v1.ObjectReference) string {
	if obj.Namespace != "" {
		return fmt.Sprintf("%s %s/%s", obj.Kind, obj.Namespace, obj.Name)
	}

	return fmt.Sprintf("%s %s", obj.Kind, obj.Name)
}

func init() {
	hostProviderIDCache = cache.New(time.Hour, time.Hour)
}

func getEventSource(controllerName string, sourceComponent string) string {
	if !ddConfig.Datadog().GetBool("kubernetes_events_source_detection.enabled") {
		return kubernetesEventSource
	}

	if v, ok := controllerToIntegration[controllerName]; ok {
		return v
	}
	if v, ok := controllerToIntegration[sourceComponent]; ok {
		return v
	}
	return defaultEventSource
}
