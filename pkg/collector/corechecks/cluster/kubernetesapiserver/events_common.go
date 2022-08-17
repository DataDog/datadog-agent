// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	hostProviderIDMutex sync.Mutex
	hostProviderIDCache map[string]string
)

type eventHostInfo struct {
	hostname   string
	nodename   string
	providerID string
}

// getDDAlertType converts kubernetes event types into datadog alert types
func getDDAlertType(k8sType string) metrics.EventAlertType {
	switch k8sType {
	case v1.EventTypeNormal:
		return metrics.EventAlertTypeInfo
	case v1.EventTypeWarning:
		return metrics.EventAlertTypeWarning
	default:
		log.Debugf("Unknown event type '%s'", k8sType)
		return metrics.EventAlertTypeInfo
	}
}

func getInvolvedObjectTags(involvedObject v1.ObjectReference) []string {
	tags := []string{
		fmt.Sprintf("kubernetes_kind:%s", involvedObject.Kind),
		fmt.Sprintf("name:%s", involvedObject.Name),
	}

	if involvedObject.Namespace != "" {
		tags = append(tags,
			// TODO remove the deprecated namespace tag, we should
			// only rely on kube_namespace
			fmt.Sprintf("namespace:%s", involvedObject.Namespace),
			fmt.Sprintf("kube_namespace:%s", involvedObject.Namespace),
		)
	}

	kindTag := getKindTag(involvedObject.Kind, involvedObject.Name)
	if kindTag != "" {
		tags = append(tags, kindTag)
	}

	return tags
}

func getEventHostInfo(clusterName string, ev *v1.Event) eventHostInfo {
	info := eventHostInfo{}

	if ev.InvolvedObject.Kind != "Pod" && ev.InvolvedObject.Kind != "Node" {
		return info
	}

	info.nodename = ev.Source.Host
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
	hostProviderIDMutex.Lock()
	defer hostProviderIDMutex.Unlock()

	if hostProviderID, hit := hostProviderIDCache[nodename]; hit {
		return hostProviderID
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

	hostProviderIDCache[nodename] = hostProviderID

	return hostProviderID
}

// getKindTag returns the kube_<kind>:<name> tag. The exact same tag names and
// object kinds are supported by the tagger. It returns an empty string if the
// kind doesn't correspond to a known/supported kind tag.
func getKindTag(kind, name string) string {
	if tagName, found := kubernetes.KindToTagName[kind]; found {
		return fmt.Sprintf("%s:%s", tagName, name)
	}
	return ""
}

func buildReadableKey(obj v1.ObjectReference) string {
	if obj.Namespace != "" {
		return fmt.Sprintf("%s %s/%s", obj.Kind, obj.Namespace, obj.Name)
	}

	return fmt.Sprintf("%s %s", obj.Kind, obj.Name)
}

func init() {
	hostProviderIDCache = make(map[string]string)
}
