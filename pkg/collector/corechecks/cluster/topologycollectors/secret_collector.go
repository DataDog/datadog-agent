// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// SecretCollector implements the ClusterTopologyCollector interface.
type SecretCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewSecretCollector
func NewSecretCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &SecretCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *SecretCollector) GetName() string {
	return "Secret Collector"
}

// Collects and Published the Secret Components
func (cmc *SecretCollector) CollectorFunction() error {
	secrets, err := cmc.GetAPIClient().GetSecrets()
	if err != nil {
		return err
	}

	for _, cm := range secrets {
		cmc.ComponentChan <- cmc.secretToStackStateComponent(cm)
	}

	return nil
}

// Creates a StackState Secret component from a Kubernetes / OpenShift Cluster
func (cmc *SecretCollector) secretToStackStateComponent(secret v1.Secret) *topology.Component {
	log.Tracef("Mapping Secret to StackState component: %s", secret.String())

	tags := cmc.initTags(secret.ObjectMeta)
	secretExternalID := cmc.buildSecretExternalID(secret.Namespace, secret.Name)

	component := &topology.Component{
		ExternalID: secretExternalID,
		Type:       topology.Type{Name: "secret"},
		Data: map[string]interface{}{
			"name":              secret.Name,
			"creationTimestamp": secret.CreationTimestamp,
			"tags":              tags,
			"uid":               secret.UID,
			"identifiers":       []string{secretExternalID},
		},
	}

	component.Data.PutNonEmpty("generateName", secret.GenerateName)
	component.Data.PutNonEmpty("kind", secret.Kind)
	component.Data.PutNonEmpty("data", secret.Data)

	log.Tracef("Created StackState Secret component %s: %v", secretExternalID, component.JSONString())

	return component
}
