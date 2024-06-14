// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/config"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func installKubernetesMetadataEndpoints(r *mux.Router, wmeta workloadmeta.Component) {
	r.HandleFunc("/annotations/node/{nodeName}", api.WithTelemetryWrapper(
		"getNodeAnnotations",
		func(w http.ResponseWriter, r *http.Request) { getNodeAnnotations(w, r, wmeta) },
	)).Methods("GET")
	r.HandleFunc("/tags/pod/{nodeName}/{ns}/{podName}", api.WithTelemetryWrapper("getPodMetadata", getPodMetadata)).Methods("GET")
	r.HandleFunc("/tags/pod/{nodeName}", api.WithTelemetryWrapper("getPodMetadataForNode", getPodMetadataForNode)).Methods("GET")
	r.HandleFunc("/tags/pod", api.WithTelemetryWrapper("getAllMetadata", getAllMetadata)).Methods("GET")
	r.HandleFunc("/tags/node/{nodeName}", api.WithTelemetryWrapper(
		"getNodeLabels",
		func(w http.ResponseWriter, r *http.Request) { getNodeLabels(w, r, wmeta) },
	)).Methods("GET")
	r.HandleFunc("/tags/namespace/{ns}", api.WithTelemetryWrapper("getNamespaceLabels", func(w http.ResponseWriter, r *http.Request) { getNamespaceLabels(w, r, wmeta) })).Methods("GET")
	r.HandleFunc("/metadata/namespace/{ns}", api.WithTelemetryWrapper("getNamespaceMetadata", func(w http.ResponseWriter, r *http.Request) { getNamespaceMetadata(w, r, wmeta) })).Methods("GET")
	r.HandleFunc("/cluster/id", api.WithTelemetryWrapper("getClusterID", getClusterID)).Methods("GET")
}

//nolint:revive // TODO(CINT) Fix revive linter
func installCloudFoundryMetadataEndpoints(r *mux.Router) {}

// getNodeMetadata is only used when the node agent hits the DCA for the list of labels or annotations
func getNodeMetadata(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component, f func(*workloadmeta.KubernetesMetadata) map[string]string, what string, filterList []string) {
	/*
		Input
			localhost:5001/api/v1/tags/node/localhost
		Outputs
			Status: 200
			Returns: []string
			Example: ["key1:value1", "key2:value2"]

			Status: 404
			Returns: string
			Example: 404 page not found

			Status: 500
			Returns: string
			Example: "no cached metadata found for the node localhost"
	*/

	vars := mux.Vars(r)
	var dataBytes []byte
	nodeName := vars["nodeName"]

	entityID := util.GenerateKubeMetadataEntityID("nodes", "", nodeName)
	nodeMetadata, err := wmeta.GetKubernetesMetadata(entityID)
	if err != nil {
		log.Errorf("Could not retrieve the node %s of %s: %v", what, nodeName, err.Error()) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nodeData := f(nodeMetadata)

	// Filter data to avoid returning too big useless data
	if filterList != nil {
		newNodeData := make(map[string]string)
		for _, key := range filterList {
			if value, found := nodeData[key]; found {
				newNodeData[key] = value
			}
		}
		nodeData = newNodeData
	}

	dataBytes, err = json.Marshal(nodeData)
	if err != nil {
		log.Errorf("Could not process the %s of the node %s from the informer's cache: %v", what, nodeName, err.Error()) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(dataBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(dataBytes)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "Could not find %s on the node: %s", what, nodeName)
}

func getNodeLabels(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	getNodeMetadata(w, r, wmeta, func(km *workloadmeta.KubernetesMetadata) map[string]string { return km.Labels }, "labels", nil)
}

func getNodeAnnotations(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	getNodeMetadata(w, r, wmeta, func(km *workloadmeta.KubernetesMetadata) map[string]string { return km.Annotations }, "annotations", config.Datadog().GetStringSlice("kubernetes_node_annotations_as_host_aliases"))
}

// getNamespaceMetadataWithTransformerFunc is used when the node agent hits the DCA for some (or all) metadata of a specific namespace
// ATTENTION: T should be marshable to json
func getNamespaceMetadataWithTransformerFunc[T any](w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component, f func(*workloadmeta.KubernetesMetadata) T, what string) {
	vars := mux.Vars(r)
	var metadataBytes []byte
	nsName := vars["ns"]
	namespaceMetadata, err := wmeta.GetKubernetesMetadata(fmt.Sprintf("namespaces//%s", nsName))
	if err != nil {
		log.Debugf("Could not retrieve the %s of namespace %s: %v", what, nsName, err.Error()) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	metadata := f(namespaceMetadata)
	metadataBytes, err = json.Marshal(metadata)
	if err != nil {
		log.Errorf("Failed to marshal %s %+v of namespace %s from the workload metadata store: %v", what, metadata, nsName, err.Error()) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(metadataBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(metadataBytes)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "Could not find %s of the namespace: %s", what, nsName)
}

// getNamespaceLabels is only used when the node agent hits the DCA for the list of labels
func getNamespaceLabels(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	/*
		Input
			localhost:5001/api/v1/tags/namespace/default
		Outputs
			Status: 200
			Returns: []string
			Example: ["label1:value1", "label2:value2"]

			Status: 404
			Returns: string
			Example: 404 page not found

			Status: 500
			Returns: string
			Example: "no cached labels found for the namespace default"
	*/
	getNamespaceMetadataWithTransformerFunc(w, r, wmeta, func(namespaceMetadata *workloadmeta.KubernetesMetadata) map[string]string {
		return namespaceMetadata.Labels
	}, "labels")
}

// getNamespaceMetadata is used when the node agent hits the DCA metadata (annotations and labels) of a specific namespace
func getNamespaceMetadata(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	/*
		Input
			localhost:5001/api/v1/metadata/namespace/default
		Outputs
			Status: 200
			Returns: []string
			Example: { labels { "key": "value" }, annotations { "key": "value" }, name "default", namespace ""}

			Status: 404
			Returns: string
			Example: 404 page not found

			Status: 500
			Returns: string
			Example: "no cached metadata found for the namespace default"
	*/
	getNamespaceMetadataWithTransformerFunc(w, r, wmeta, func(namespaceMetadata *workloadmeta.KubernetesMetadata) workloadmeta.EntityMeta {
		return namespaceMetadata.EntityMeta
	}, "metadata")
}

// getPodMetadata is only used when the node agent hits the DCA for the tags list.
// It returns a list of all the tags that can be directly used in the tagger of the agent.
func getPodMetadata(w http.ResponseWriter, r *http.Request) {
	/*
		Input
			localhost:5001/api/v1/metadata/localhost/default/my-nginx-5d69
		Outputs
			Status: 200
			Returns: []string
			Example: ["kube_service:my-nginx-service"]

			Status: 404
			Returns: string
			Example: 404 page not found

			Status: 500
			Returns: string
			Example: "no cached metadata found for the pod my-nginx-5d69 on the node localhost"
	*/

	vars := mux.Vars(r)
	var metaBytes []byte
	nodeName := vars["nodeName"]
	podName := vars["podName"]
	ns := vars["ns"]
	metaList, errMetaList := controllers.GetPodMetadataNames(nodeName, ns, podName)
	if errMetaList != nil {
		log.Errorf("Could not retrieve the metadata of: %s from the cache", podName) //nolint:errcheck
		http.Error(w, errMetaList.Error(), http.StatusInternalServerError)
		return
	}

	metaBytes, err := json.Marshal(metaList)
	if err != nil {
		log.Errorf("Could not process the list of services for: %s", podName) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(metaBytes) != 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(metaBytes)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "Could not find associated metadata mapped to the pod: %s on node: %s", podName, nodeName)
}

// getPodMetadataForNode has the same signature as getAllMetadata, but is only scoped on one node.
func getPodMetadataForNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeName := vars["nodeName"]
	log.Tracef("Fetching metadata map on all pods of the node %s", nodeName)
	metaList, errNodes := as.GetMetadataMapBundleOnNode(nodeName)
	if errNodes != nil {
		log.Warnf("Could not collect the service map for %s, err: %v", nodeName, errNodes) //nolint:errcheck
	}
	slcB, err := json.Marshal(metaList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(slcB) != 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(slcB)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// getAllMetadata is used by the svcmap command.
//
//nolint:revive // TODO(CINT) Fix revive linter
func getAllMetadata(w http.ResponseWriter, r *http.Request) {
	/*
		Input
			localhost:5001/api/v1/metadata
		Outputs
			Status: 200
			Returns: map[string][]string
			Example: ["Node1":["pod1":["svc1"],"pod2":["svc2"]],"Node2":["pod3":["svc1"]], "Error":"the key KubernetesMetadataMapping/Node3 not found in the cache"]

			Status: 404
			Returns: string
			Example: 404 page not found

			Status: 503
			Returns: map[string]string
			Example: "["Error":"could not collect the service map for all nodes: List services is not permitted at the cluster scope."]
	*/
	log.Trace("Computing metadata map on all nodes")
	// As HTTP query handler, we do not retry getting the APIServer
	// Client will have to retry query in case of failure
	cl, err := as.GetAPIClient()
	if err != nil {
		log.Errorf("Can't create client to query the API Server: %v", err) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	metaList, errAPIServer := as.GetMetadataMapBundleOnAllNodes(cl)
	// If we hit an error at this point, it is because we don't have access to the API server.
	if errAPIServer != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Errorf("There was an error querying the nodes from the API: %s", errAPIServer.Error()) //nolint:errcheck
	} else {
		w.WriteHeader(http.StatusOK)
	}
	metaListBytes, err := json.Marshal(metaList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(metaListBytes) != 0 {
		w.Write(metaListBytes)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// getClusterID is used by recent agents to get the cluster UUID, needed for enabling the orchestrator explorer
//
//nolint:revive // TODO(CINT) Fix revive linter
func getClusterID(w http.ResponseWriter, r *http.Request) {
	// As HTTP query handler, we do not retry getting the APIServer
	// Client will have to retry query in case of failure
	cl, err := as.GetAPIClient()
	if err != nil {
		log.Errorf("Can't create client to query the API Server: %v", err) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	coreCl := cl.Cl.CoreV1().(*corev1.CoreV1Client)
	// get clusterID
	clusterID, err := apicommon.GetOrCreateClusterID(coreCl)
	if err != nil {
		log.Errorf("Failed to generate or retrieve the cluster ID: %v", err) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// write response
	j, err := json.Marshal(clusterID)
	if err != nil {
		log.Errorf("Failed to marshal the cluster ID: %v", err) //nolint:errcheck
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(j)
}
