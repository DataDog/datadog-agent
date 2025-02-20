// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// NodeMetadataMapping only fetch the endpoints from Kubernetes apiserver and add the metadataMapper of the
// node to the cache
// Only called when the node agent computes the metadata mapper locally and does not rely on the DCA.
func (c *APIClient) NodeMetadataMapping(nodeName string, pods []*kubelet.Pod) error {
	/*
		endpointList, err := c.Cl.CoreV1().Endpoints("").List(context.TODO(), metav1.ListOptions{TimeoutSeconds: pointer.Ptr(int64(c.defaultClientTimeout.Seconds())), ResourceVersion: "0"})
		if err != nil {
			log.Errorf("Could not collect endpoints from the API Server: %q", err.Error())
			return err
		}
		if endpointList.Items == nil {
			log.Debug("No endpoints collected from the API server")
			return nil
		}
		log.Debugf("Successfully collected endpoints")

		var node v1.Node
		var nodeList v1.NodeList
		node.Name = nodeName

		nodeList.Items = append(nodeList.Items, node)

		processKubeServices(&nodeList, pods, endpointList)

	*/
	return nil
}
