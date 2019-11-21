// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// ConfigMapCollector implements the ClusterTopologyCollector interface.
type ConfigMapCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewConfigMapCollector
func NewConfigMapCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ConfigMapCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *ConfigMapCollector) GetName() string {
	return "ConfigMap Collector"
}

// Collects and Published the ConfigMap Components
func (cmc *ConfigMapCollector) CollectorFunction() error {
	configMaps, err := cmc.GetAPIClient().GetConfigMaps()
	if err != nil {
		return err
	}

	for _, cm := range configMaps {
		cmc.ComponentChan <- cmc.configMapToStackStateComponent(cm)
	}

	return nil
}

// Creates a StackState ConfigMap component from a Kubernetes / OpenShift Cluster
func (cmc *ConfigMapCollector) configMapToStackStateComponent(configMap v1.ConfigMap) *topology.Component {
	log.Tracef("Mapping ConfigMap to StackState component: %s", configMap.String())

	tags := emptyIfNil(configMap.Labels)
	tags = cmc.addClusterNameTag(tags)

	configMapExternalID := cmc.buildConfigMapExternalID(configMap.Namespace, configMap.Name)

	component := &topology.Component{
		ExternalID: configMapExternalID,
		Type:       topology.Type{Name: "configmap"},
		Data: map[string]interface{}{
			"name":              configMap.Name,
			"creationTimestamp": configMap.CreationTimestamp,
			"tags":              tags,
			"namespace":         configMap.Namespace,
			"uid":               configMap.UID,
			"identifiers":       []string{configMapExternalID},
		},
	}

	component.Data.PutNonEmpty("generateName", configMap.GenerateName)
	component.Data.PutNonEmpty("kind", configMap.Kind)
	component.Data.PutNonEmpty("data", configMap.Data)

	log.Tracef("Created StackState ConfigMap component %s: %v", configMapExternalID, component.JSONString())

	return component
}
