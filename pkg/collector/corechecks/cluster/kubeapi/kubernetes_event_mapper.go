// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/urn"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"

	"github.com/StackVista/stackstate-agent/pkg/util/log"

	"github.com/StackVista/stackstate-agent/pkg/metrics"
	v1 "k8s.io/api/core/v1"
)

type kubernetesEventMapper struct {
	ac          *apiserver.APIClient
	urn         urn.Builder
	clusterName string
	sourceType  string
}

func newKubernetesEventMapper(ac *apiserver.APIClient, clusterName string) *kubernetesEventMapper {
	f := kubernetesFlavour(ac)
	return &kubernetesEventMapper{
		ac:          ac,
		urn:         urn.NewURNBuilder(f, clusterName),
		clusterName: clusterName,
		sourceType:  string(f),
	}
}

func (k *kubernetesEventMapper) mapKubernetesEvent(event *v1.Event, modified bool) (metrics.Event, error) {
	if err := checkEvent(event); err != nil {
		return metrics.Event{}, err
	}

	mEvent := metrics.Event{
		Title:          fmt.Sprintf("%s - %s %s (%dx)", event.Reason, event.InvolvedObject.Name, event.InvolvedObject.Kind, event.Count),
		Host:           getHostName(event, k.clusterName),
		SourceTypeName: k.sourceType,
		Priority:       metrics.EventPriorityNormal,
		AlertType:      getAlertType(event),
		EventType:      event.Reason,
		Ts:             getTimeStamp(event, modified),
		Tags:           k.getTags(event),
		EventContext: &metrics.EventContext{
			Source:           k.sourceType,
			Category:         k.sourceType,
			SourceIdentifier: string(event.GetUID()),
			ElementIdentifiers: []string{
				k.externalIdentifierForInvolvedObject(event),
			},
		},
		Text: event.Message,
	}

	return mEvent, nil
}

func checkEvent(event *v1.Event) error {
	// As some fields are optional, we want to avoid evaluating empty values.
	if event == nil || event.InvolvedObject.Kind == "" {
		return errors.New("could not retrieve some parent attributes of the event")
	}

	if event.Reason == "" || event.Message == "" || event.InvolvedObject.Name == "" {
		return errors.New("could not retrieve some attributes of the event")
	}

	return nil
}

func getHostName(event *v1.Event, clusterName string) string {
	if event.InvolvedObject.Kind == "Node" || event.InvolvedObject.Kind == "Pod" {
		if clusterName != "" {
			return fmt.Sprintf("%s-%s", event.Source.Host, clusterName)
		}

		return event.Source.Host
	}

	// If hostname was not defined, the aggregator will then set the local hostname
	return ""
}

func getAlertType(event *v1.Event) metrics.EventAlertType {
	switch strings.ToLower(event.Type) {
	case "normal":
		return metrics.EventAlertTypeInfo
	case "warning":
		return metrics.EventAlertTypeWarning
	default:
		log.Warnf("Unhandled kubernetes event type '%s', fallback to metrics.EventAlertTypeInfo", event.Type)
		return metrics.EventAlertTypeInfo
	}
}

func getTimeStamp(event *v1.Event, modified bool) int64 {
	if modified {
		return event.LastTimestamp.Unix()
	}

	return event.FirstTimestamp.Unix()
}

func (k *kubernetesEventMapper) getTags(event *v1.Event) []string {
	tags := []string{}

	if event.Namespace != "" {
		tags = append(tags, fmt.Sprintf("kube_namespace:%s", event.Namespace))
	}

	tags = append(tags, fmt.Sprintf("source_component:%s", event.Source.Component))
	tags = append(tags, fmt.Sprintf("kube_object_name:%s", event.InvolvedObject.Name))
	tags = append(tags, fmt.Sprintf("kube_object_kind:%s", event.InvolvedObject.Kind))
	tags = append(tags, fmt.Sprintf("kube_cluster_name:%s", k.clusterName))
	tags = append(tags, fmt.Sprintf("kube_reason:%s", event.Reason))

	return tags
}

func (k *kubernetesEventMapper) externalIdentifierForInvolvedObject(event *v1.Event) string {
	namespace := event.Namespace
	obj := event.InvolvedObject

	urn, err := k.urn.BuildExternalID(obj.Kind, namespace, obj.Name)
	if err != nil {
		log.Warnf("Unknown InvolvedObject type '%s' for obj '%s/%s' in event '%s'", obj.Kind, namespace, obj.Name, event.Name)
		return ""
	}

	return urn
}

func kubernetesFlavour(ac *apiserver.APIClient) urn.ClusterType {
	switch openshiftPresence := ac.DetectOpenShiftAPILevel(); openshiftPresence {
	case apiserver.OpenShiftAPIGroup, apiserver.OpenShiftOAPI:
		return urn.Openshift
	default:
		return urn.Kubernetes
	}

}
