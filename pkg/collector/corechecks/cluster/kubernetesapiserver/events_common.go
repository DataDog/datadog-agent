// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/patrickmn/go-cache"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/kubetags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	hostProviderIDCache *cache.Cache
	cronJobByJobCache   *cache.Cache
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

// customEventSourceSuffix is the suffix that will be added to the event source type when
// filtering is enabled and the event does not exist within integrationToCollectedEventTypes.
const customEventSourceSuffix = "custom"

var integrationToCollectedEventTypes = map[string][]collectedEventType{
	"kubernetes": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"kube_scheduler": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"kubernetes controller manager": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"karpenter": {
		{
			Source: "karpenter",
			Reasons: []string{
				"DisruptionBlocked",
				"DisruptionLaunching",
				"DisruptionTerminating",
				"DisruptionWaitingReadiness",
				"FailedDraining",
				"InstanceTerminating",
				"SpotInterrupted",
				"SpotRebalanceRecommendation",
				"TerminatingOnInterruption",
			},
		},
	},
	"datadog-operator": {
		{
			Source: "datadog-operator",
		},
	},
	"amazon elb": {
		{
			Source: "amazon elb",
		},
	},
	"cilium": {
		{
			Source: "cilium",
		},
	},
	"fluxcd": {
		{
			Source: "fluxcd",
		},
	},
	"kubernetes cluster autoscaler": {
		{

			Source: "kubernetes cluster autoscaler",
		},
	},
	"spark": {
		{
			Source: "spark",
		},
	},
	"vault": {

		{
			Source: "vault",
		},
	},
	"default": {
		{
			Reasons: []string{"BackOff"}, // Change tracking consumes all CLB events
		},
	},
}

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

// cronJobResolverFunc resolves the name of the CronJob owning the given Job,
// or returns an empty string if the Job has no CronJob owner.
type cronJobResolverFunc func(namespace, jobName, uid string) string

func getInvolvedObjectTags(involvedObject v1.ObjectReference, taggerInstance tagger.Component) []string {
	return getInvolvedObjectTagsImpl(involvedObject, taggerInstance, getCronJobForJob)
}

// getInvolvedObjectTagsImpl takes a `resolveCronJobForJob` function to ease
// unit-testing by mocking the API server lookup
func getInvolvedObjectTagsImpl(involvedObject v1.ObjectReference, taggerInstance tagger.Component, resolveCronJobForJob cronJobResolverFunc) []string {
	// NOTE: we now standardized on using kube_* tags, instead of
	// non-namespaced ones, or kubernetes_*. The latter two are now
	// considered deprecated.
	tagList := []string{
		"kube_kind:" + involvedObject.Kind,
		"kube_name:" + involvedObject.Name,

		// DEPRECATED:
		"kubernetes_kind:" + involvedObject.Kind,
		"name:" + involvedObject.Name,
	}

	if involvedObject.Namespace != "" {
		tagList = append(tagList,
			"kube_namespace:"+involvedObject.Namespace,

			// DEPRECATED:
			"namespace:"+involvedObject.Namespace,
		)

		namespaceEntityID := types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", involvedObject.Namespace)))
		namespaceEntity, err := taggerInstance.GetEntity(namespaceEntityID)
		if err == nil {
			tagList = append(tagList, namespaceEntity.GetTags(types.HighCardinality)...)
		}
	}

	var entityID types.EntityID

	switch involvedObject.Kind {
	case podKind:
		entityID = types.NewEntityID(types.KubernetesPodUID, string(involvedObject.UID))
	case deploymentKind:
		entityID = types.NewEntityID(types.KubernetesDeployment, involvedObject.Namespace+"/"+involvedObject.Name)
	default:
		apiGroup := apiserver.GetAPIGroup(involvedObject.APIVersion)
		resourceType, err := apiserver.GetResourceType(involvedObject.Kind, apiGroup)
		if err != nil {
			log.Debugf("error getting resource type for kind '%s' and group '%s', tags may be missing: %v", involvedObject.Kind, apiGroup, err)
		}
		entityID = types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID(apiGroup, resourceType, involvedObject.Namespace, involvedObject.Name)))
	}

	entity, err := taggerInstance.GetEntity(entityID)
	if err == nil {
		tagList = append(tagList, entity.GetTags(types.HighCardinality)...)
	} else {
		log.Debugf("error getting entity for entity ID '%s': tags may be missing", entityID)
	}

	kindTag := getKindTag(involvedObject.Kind, involvedObject.Name)
	if kindTag != "" {
		tagList = append(tagList, kindTag)
	}

	// Jobs created by a CronJob don't get the kube_cronjob tag from the
	// KubernetesMetadata tagger entity (ownerReferences aren't resolved there,
	// unlike the Pod path), so resolve it from the Job's ownerReferences.
	if involvedObject.Kind == jobKind {
		if cronJob := resolveCronJobForJob(involvedObject.Namespace, involvedObject.Name, string(involvedObject.UID)); cronJob != "" {
			tagList = append(tagList, "kube_cronjob:"+cronJob)
		}
	}

	return tagList
}

// getCronJobForJob returns the name of the CronJob owning the given Job, or an
// empty string if the Job has no CronJob owner or cannot be fetched from the
// API server. Results are cached to avoid repeated API calls for the same Job.
func getCronJobForJob(namespace, name, uid string) string {
	// A Job created by a CronJob is always named "<cronjob>-<timestamp>". If the
	// name doesn't match that pattern, the Job can't have a CronJob owner, so we
	// skip the API lookup entirely (this is how the Pod and KSM paths derive the
	// tag, see kubernetes.ParseCronJobForJob).
	if cronJob, _ := kubernetes.ParseCronJobForJob(name); cronJob == "" {
		return ""
	}

	// Key the cache by the Job's UID (globally unique) so that a Job deleted and
	// recreated with the same name doesn't return a stale result. This matches
	// how the rest of the check identifies the involved object (e.g. the event
	// AggregationKey and the KubernetesPodUID tagger entity). Events aren't
	// guaranteed to carry the involved object's UID; when it's absent (rare) we
	// skip the cache to avoid distinct Jobs sharing an empty-string key.
	if uid != "" {
		if cronJob, hit := cronJobByJobCache.Get(uid); hit {
			return cronJob.(string)
		}
	}

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		log.Warnf("Can't create client to query the API Server: %v", err)
		return ""
	}

	cronJob, definitive := getCronJobForJobWithClient(context.TODO(), cl.Cl, namespace, name, uid)
	if uid != "" && definitive {
		cronJobByJobCache.Set(uid, cronJob, cache.DefaultExpiration)
	}

	return cronJob
}

// getCronJobForJobWithClient fetches the Job from the API server and returns
// the name of its CronJob controller, or an empty string if it has none. The
// returned boolean reports whether the result is definitive: transient errors
// return false so that the caller doesn't cache the empty result.
func getCronJobForJobWithClient(ctx context.Context, client kubeclient.Interface, namespace, name, uid string) (string, bool) {
	job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			// The Job may legitimately be gone by the time the event is
			// processed (e.g. ttlSecondsAfterFinished).
			log.Debugf("Job %s/%s not found, kube_cronjob tag may be missing: %v", namespace, name, err)
			return "", true
		case apierrors.IsForbidden(err):
			// Permanent error (e.g. missing RBAC): report the empty result as
			// definitive so the caller caches it and we don't re-query on every
			// event of the same Job.
			log.Debugf("not allowed to get job %s/%s, kube_cronjob tag may be missing: %v", namespace, name, err)
			return "", true
		default:
			log.Debugf("error getting job %s/%s, kube_cronjob tag may be missing: %v", namespace, name, err)
			return "", false
		}
	}

	// Guard against Job name reuse: if the Job was deleted and recreated with the
	// same name, the fetched object's UID won't match the event's, and its
	// ownerReferences don't apply to this event.
	if uid != "" && string(job.UID) != uid {
		log.Debugf("Job %s/%s UID mismatch (event=%s, fetched=%s), kube_cronjob tag may be missing", namespace, name, uid, job.UID)
		return "", true
	}

	if ref := metav1.GetControllerOf(job); ref != nil &&
		ref.Kind == kubernetes.CronJobKind &&
		strings.HasPrefix(ref.APIVersion, "batch/") {
		return ref.Name, true
	}

	return "", true
}

const (
	podKind        = "Pod"
	nodeKind       = "Node"
	deploymentKind = "Deployment"
	jobKind        = "Job"
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
		sourceHost := ev.Source.Host
		if sourceHost != "" {
			info.nodename = sourceHost
			break
		}
		c, err := apiserver.GetAPIClient()
		if err == nil {
			ctx := context.TODO()
			node, err := c.GetNodeForPod(ctx, ev.InvolvedObject.Namespace, ev.InvolvedObject.Name)
			if err == nil {
				sourceHost = node
			}
		}
		info.nodename = sourceHost
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
	tagName, err := kubetags.GetTagForKubernetesKind(kind)
	if err != nil {
		return ""
	}

	return tagName + ":" + name
}

func buildReadableKey(obj v1.ObjectReference) string {
	if obj.Namespace != "" {
		return fmt.Sprintf("%s %s/%s", obj.Kind, obj.Namespace, obj.Name)
	}

	return fmt.Sprintf("%s %s", obj.Kind, obj.Name)
}

func init() {
	hostProviderIDCache = cache.New(time.Hour, time.Hour)
	// CronJob-owned Job names change on every run, so cache entries are never
	// reused: use a short TTL to keep the cache size bounded.
	cronJobByJobCache = cache.New(30*time.Minute, 10*time.Minute)
}

func getEventSource(controllerName string, sourceComponent string) string {
	if !pkgconfigsetup.Datadog().GetBool("kubernetes_events_source_detection.enabled") {
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

func shouldCollectByDefault(ev *v1.Event) bool {
	if v, ok := integrationToCollectedEventTypes[getEventSource(ev.ReportingController, ev.Source.Component)]; ok {
		return shouldCollect(ev, append(v, integrationToCollectedEventTypes["default"]...))
	}
	return shouldCollect(ev, integrationToCollectedEventTypes["default"])
}

func shouldCollect(ev *v1.Event, collectedTypes []collectedEventType) bool {
	involvedObject := ev.InvolvedObject

	for _, f := range collectedTypes {
		if f.Kind != "" && f.Kind != involvedObject.Kind {
			continue
		}

		if f.Source != "" && f.Source != ev.Source.Component {
			continue
		}

		if len(f.Reasons) == 0 {
			return true
		}

		if slices.Contains(f.Reasons, ev.Reason) {
			return true
		}
	}

	return false
}
