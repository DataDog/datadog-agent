// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package v1

import (
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/paulbellamy/ratecounter"

	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	apiStats              = expvar.NewMap("apiv1")
	tagsStats             = new(expvar.Map).Init()
	tagsErrors            = &expvar.Int{}
	tagsRequestsPerSecond = &expvar.Int{}
	tagsRequestsCounter   = ratecounter.NewRateCounter(1 * time.Second)
)

func init() {
	apiStats.Set("Tags", tagsStats)
	tagsStats.Set("Errors", tagsErrors)
	tagsStats.Set("RequestsPerSecond", tagsRequestsPerSecond)
}

// Install registers v1 API endpoints
func Install(r *mux.Router) {
	r.HandleFunc("/tags/{nodeName}/{ns}/{podName}", getPodTags).Methods("GET")
	r.HandleFunc("/metadata/{nodeName}", getNodeMetadata).Methods("GET")
	r.HandleFunc("/metadata", getAllMetadata).Methods("GET")
}

// getPodTags returns a list of cluster-level tags for the specified pod, namespace, and node.
func getPodTags(w http.ResponseWriter, r *http.Request) {
	/*
		Input
			localhost:5001/api/v1/tags/localhost/default/my-nginx-5d69
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
	tagsRequestsCounter.Incr(1)
	tagsRequestsPerSecond.Set(tagsRequestsCounter.Rate())

	vars := mux.Vars(r)
	nodeName := vars["nodeName"]
	podName := vars["podName"]
	ns := vars["ns"]
	metaList, errMetaList := as.GetPodClusterTags(nodeName, ns, podName)
	if errMetaList != nil {
		log.Errorf("Could not retrieve the metadata of: %s from the cache", podName)
		http.Error(w, errMetaList.Error(), http.StatusInternalServerError)
		tagsErrors.Add(1)
		return
	}

	metaBytes, err := json.Marshal(metaList)
	if err != nil {
		log.Errorf("Could not process the list of services for: %s", podName)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		tagsErrors.Add(1)
		return
	}
	if len(metaBytes) != 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(metaBytes)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(fmt.Sprintf("Could not find associated metadata mapped to the pod: %s on node: %s", podName, nodeName)))
}

// getNodeMetadata has the same signature as getAllMetadata, but is only scoped on one node.
func getNodeMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodeName := vars["nodeName"]
	log.Infof("Fetching metadata map on all pods of the node %s", nodeName)
	metaList, errNodes := as.GetMetadataMapBundleOnNode(nodeName)
	if errNodes != nil {
		log.Errorf("Could not collect the service map for %s", nodeName)
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
	return
}

// getAllMetadata is used by the svcmap command.
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
	log.Info("Computing metadata map on all nodes")
	cl, err := as.GetAPIClient()
	if err != nil {
		log.Errorf("Can't create client to query the API Server: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	metaList, errAPIServer := as.GetMetadataMapBundleOnAllNodes(cl)
	// If we hit an error at this point, it is because we don't have access to the API server.
	if errAPIServer != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Errorf("There was an error querying the nodes from the API: %s", errAPIServer.Error())
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
	return
}
