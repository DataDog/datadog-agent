// +build kubeapiserver

package topologycollectors

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// SecretCollector implements the ClusterTopologyCollector interface.
type SecretCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewSecretCollector creates a new instance of the secret collector
func NewSecretCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &SecretCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (*SecretCollector) GetName() string {
	return "Secret Collector"
}

// CollectorFunction Collects and Published the Secret Components
func (cmc *SecretCollector) CollectorFunction() error {
	secrets, err := cmc.GetAPIClient().GetSecrets()
	if err != nil {
		return err
	}

	for _, cm := range secrets {
		comp, err := cmc.secretToStackStateComponent(cm)
		if err != nil {
			return err
		}

		cmc.ComponentChan <- comp
	}

	return nil
}

// Creates a StackState Secret component from a Kubernetes / OpenShift Cluster
func (cmc *SecretCollector) secretToStackStateComponent(secret v1.Secret) (*topology.Component, error) {
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

	hash, err := secure(secret.Data)
	if err != nil {
		return nil, err
	}
	component.Data.PutNonEmpty("data", hash)

	log.Tracef("Created StackState Secret component %s: %v", secretExternalID, component.JSONString())

	return component, nil
}

func secure(data map[string][]byte) (string, error) {
	hash := sha256.New()
	if len(data) == 0 {
		return hex.EncodeToString(hash.Sum(nil)), nil
	}

	k := keys(data)
	sort.Strings(k) // Sort so that we have a stable hash

	for _, key := range k {
		if _, err := hash.Write([]byte(key)); err != nil {
			return "", err
		}

		val := data[key]
		if _, err := hash.Write(val); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func keys(data map[string][]byte) []string {
	keys := make([]string, len(data))
	i := 0

	for k := range data {
		keys[i] = k
		i++
	}

	return keys
}
