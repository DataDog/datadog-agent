// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build clusterchecks,!kubeapiserver

package v1

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
)

func installCloudFoundryMetadataEndpoints(r *mux.Router) {
	r.HandleFunc("/tags/cf/apps/{nodeName}", getCFAppsMetadataForNode).Methods("GET")
}

func installKubernetesMetadataEndpoints(r *mux.Router) {}

// getCFAppsMetadataForNode is only used when the node agent hits the DCA for the list of cloudfoundry applications tags
// It return a list of tags for each application that can be directly used in the tagger
func getCFAppsMetadataForNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodename := vars["nodeName"]
	bbsCache, err := cloudfoundry.GetGlobalBBSCache()
	if err != nil {
		log.Errorf("Could not retrieve BBS cache: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		apiRequests.Inc("getCFAppsMetadataForNode", strconv.Itoa(http.StatusInternalServerError))
		return
	}

	tags, err := bbsCache.GetTagsForNode(nodename)
	if err != nil {
		log.Errorf("Error getting tags for node %s: %v", nodename, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		apiRequests.Inc(
			"getCFAppsMetadataForNode",
			strconv.Itoa(http.StatusInternalServerError),
		)
		return
	}

	tagsBytes, err := json.Marshal(tags)
	if err != nil {
		log.Errorf("Could not process tags for CF applications: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		apiRequests.Inc(
			"getCFAppsMetadataForNode",
			strconv.Itoa(http.StatusInternalServerError),
		)
		return
	}
	if len(tagsBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(tagsBytes)
		apiRequests.Inc(
			"getCFAppsMetadataForNode",
			strconv.Itoa(http.StatusOK),
		)
		return
	}
}
