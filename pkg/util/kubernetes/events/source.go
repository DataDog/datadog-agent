// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

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

// CustomSourceSuffix is the suffix that is added to the event source type when
// filtering is enabled and the event does not match the default collected event types.
const CustomSourceSuffix = "custom"

// SourceType returns the integration a Kubernetes event is attributed to, used as the
// source_type_name of the Datadog event.
//
// When detectionEnabled is false every event is attributed to "kubernetes".
// Otherwise the reporting controller is looked up first, then the source component, which
// is all that older emitters set. Events from an unknown emitter are attributed to
// "kubernetes" as well.
func SourceType(reportingController, sourceComponent string, detectionEnabled bool) string {
	if !detectionEnabled {
		return kubernetesEventSource
	}

	if v, ok := controllerToIntegration[reportingController]; ok {
		return v
	}
	if v, ok := controllerToIntegration[sourceComponent]; ok {
		return v
	}
	return defaultEventSource
}
