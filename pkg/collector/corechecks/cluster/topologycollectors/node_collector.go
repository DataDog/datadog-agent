// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	"strings"
)

// NodeCollector implements the ClusterTopologyCollector interface.
type NodeCollector struct {
	ComponentChan          chan<- *topology.Component
	RelationChan           chan<- *topology.Relation
	NodeIdentifierCorrChan chan<- *NodeIdentifierCorrelation
	ClusterTopologyCollector
}

// NodeStatus is the StackState representation of a Kubernetes / Openshift Node Status
type NodeStatus struct {
	Phase           v1.NodePhase      `json:"phase,omitempty"`
	NodeInfo        v1.NodeSystemInfo `json:"nodeInfo,omitempty"`
	KubeletEndpoint v1.DaemonEndpoint `json:"kubeletEndpoint,omitempty"`
}

// NewNodeCollector
func NewNodeCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	nodeIdentifierCorrChan chan<- *NodeIdentifierCorrelation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &NodeCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		NodeIdentifierCorrChan:   nodeIdentifierCorrChan,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *NodeCollector) GetName() string {
	return "Node Collector"
}

// Collects and Published the Node Components
func (nc *NodeCollector) CollectorFunction() error {
	// get all the nodes in the cluster
	nodes, err := nc.GetAPIClient().GetNodes()
	if err != nil {
		return err
	}

	for _, node := range nodes {
		// creates and publishes StackState node component
		component := nc.nodeToStackStateComponent(node)
		// creates a StackState relation for the cluster node -> cluster
		relation := nc.nodeToClusterStackStateRelation(node)

		nc.ComponentChan <- component
		nc.RelationChan <- relation

		// send the node identifier to be correlated
		if node.Spec.ProviderID != "" {
			nodeIdentifier := extractInstanceIdFromProviderId(node.Spec)
			if nodeIdentifier != "" {
				nc.NodeIdentifierCorrChan <- &NodeIdentifierCorrelation{node.Name, nodeIdentifier}
			}
		}
	}

	close(nc.NodeIdentifierCorrChan)

	return nil
}

// Creates a StackState component from a Kubernetes Node
func (nc *NodeCollector) nodeToStackStateComponent(node v1.Node) *topology.Component {
	// creates a StackState component for the kubernetes node
	log.Tracef("Mapping kubernetes node to StackState component: %s", node.String())

	// create identifier list to merge with StackState components
	identifiers := make([]string, 0)
	for _, address := range node.Status.Addresses {
		switch addressType := address.Type; addressType {
		case v1.NodeInternalIP:
			identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s:%s", nc.GetInstance().URL, node.Name, address.Address))
		case v1.NodeExternalIP:
			identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s", nc.GetInstance().URL, address.Address))
		case v1.NodeInternalDNS:
			identifiers = append(identifiers, fmt.Sprintf("urn:host:/%s:%s", nc.GetInstance().URL, address.Address))
		case v1.NodeExternalDNS:
			identifiers = append(identifiers, fmt.Sprintf("urn:host:/%s", address.Address))
		case v1.NodeHostName:
			//do nothing with it
		default:
			continue
		}
	}
	// this allow merging with host reported by main agent
	var instanceId string
	if len(node.Spec.ProviderID) > 0 {
		instanceId = extractInstanceIdFromProviderId(node.Spec)
		identifiers = append(identifiers, fmt.Sprintf("urn:host:/%s", instanceId))
	}

	log.Tracef("Created identifiers for %s: %v", node.Name, identifiers)

	nodeExternalID := nc.buildNodeExternalID(node.Name)

	tags := emptyIfNil(node.Labels)
	tags = nc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: nodeExternalID,
		Type:       topology.Type{Name: "node"},
		Data: map[string]interface{}{
			"name":              node.Name,
			"namespace":         node.Namespace,
			"creationTimestamp": node.CreationTimestamp,
			"tags":              tags,
			"status": NodeStatus{
				Phase:           node.Status.Phase,
				NodeInfo:        node.Status.NodeInfo,
				KubeletEndpoint: node.Status.DaemonEndpoints.KubeletEndpoint,
			},
			"identifiers": identifiers,
			"uid":         node.UID,
			//"taints": node.Spec.Taints,
		},
	}

	component.Data.PutNonEmpty("generateName", node.GenerateName)
	component.Data.PutNonEmpty("kind", node.Kind)
	component.Data.PutNonEmpty("instanceId", instanceId)

	log.Tracef("Created StackState node component %s: %v", nodeExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes Pod to Node relation
func (nc *NodeCollector) nodeToClusterStackStateRelation(node v1.Node) *topology.Relation {
	nodeExternalID := nc.buildNodeExternalID(node.Name)
	clusterExternalID := nc.buildClusterExternalID()

	log.Tracef("Mapping kubernetes node to cluster relation: %s -> %s", nodeExternalID, clusterExternalID)

	relation := nc.CreateRelation(nodeExternalID, clusterExternalID, "belongs_to")

	log.Tracef("Created StackState node -> cluster relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

func emptyIfNil(m map[string]string) map[string]string {
	if m == nil {
		m = make(map[string]string, 0)
	}
	return m
}

func extractLastFragment(value string) string {
	lastSlash := strings.LastIndex(value, "/")
	return value[lastSlash+1:]
}

func extractInstanceIdFromProviderId(spec v1.NodeSpec) string {
	//parse node id from cloud provider (for AWS is the ec2 instance id)
	return extractLastFragment(spec.ProviderID)
}
