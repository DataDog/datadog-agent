// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver

package v1

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func installCloudFoundryMetadataEndpoints(r *mux.Router) {
	r.HandleFunc("/tags/cf/apps/{nodeName}", api.WithTelemetryWrapper("getCFAppsMetadataForNode", getCFAppsMetadataForNode)).Methods("GET")

	if config.Datadog.GetBool("cluster_agent.serve_nozzle_data") {
		r.HandleFunc("/cf/apps/{guid}", api.WithTelemetryWrapper("getCFApplication", getCFApplication)).Methods("GET")
		r.HandleFunc("/cf/apps", api.WithTelemetryWrapper("getCFApplications", getCFApplications)).Methods("GET")
		r.HandleFunc("/cf/org_quotas", api.WithTelemetryWrapper("getCFOrgQuotas", getCFOrgQuotas)).Methods("GET")
		r.HandleFunc("/cf/orgs", api.WithTelemetryWrapper("getCFOrgs", getCFOrgs)).Methods("GET")
	}
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
		return
	}

	tags, err := bbsCache.GetTagsForNode(nodename)
	if err != nil {
		log.Errorf("Error getting tags for node %s: %v", nodename, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tagsBytes, err := json.Marshal(tags)
	if err != nil {
		log.Errorf("Could not process tags for CF applications: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(tagsBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(tagsBytes)
		return
	}
}

// getCFApplications is only used when the PCF firehose nozzle hits the DCA for the list of cloudfoundry applications
// It return a list of CFApplications
func getCFApplications(w http.ResponseWriter, r *http.Request) {
	ccCache, err := cloudfoundry.GetGlobalCCCache()
	if err != nil {
		log.Errorf("Could not retrieve CC cache: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apps, err := ccCache.GetCFApplications()
	if err != nil {
		log.Errorf("Error getting applications: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	appsBytes, err := json.Marshal(apps)
	if err != nil {
		log.Errorf("Could not process CF applications: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(appsBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(appsBytes)
		return
	}
}

// getCFApplication is only used when the PCF firehose nozzle hits the DCA for a single cloudfoundry application
// It return a single CFApplication with the given guid
func getCFApplication(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	guid := vars["guid"]
	ccCache, err := cloudfoundry.GetGlobalCCCache()
	if err != nil {
		log.Errorf("Could not retrieve CC cache: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app, err := ccCache.GetCFApplication(guid)
	if err != nil {
		log.Errorf("Error getting application: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	appBytes, err := json.Marshal(app)
	if err != nil {
		log.Errorf("Could not process CF application: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(appBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(appBytes)
		return
	}
}

// getCFOrgs is only used when the PCF firehose nozzle hits the DCA for the list of cloudfoundry organizations
// It return a list of V3 CF Organizations
func getCFOrgs(w http.ResponseWriter, r *http.Request) {
	ccCache, err := cloudfoundry.GetGlobalCCCache()
	if err != nil {
		log.Errorf("Could not retrieve CC cache: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	orgs, err := ccCache.GetOrgs()
	if err != nil {
		log.Errorf("Error getting organization: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	orgsBytes, err := json.Marshal(orgs)
	if err != nil {
		log.Errorf("Could not process CF organization: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(orgsBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(orgsBytes)
		return
	}
}

// getCFOrgQuotas is only used when the PCF firehose nozzle hits the DCA for the list of cloudfoundry organization quotas
// It return a list of V2 CF organization quotas
func getCFOrgQuotas(w http.ResponseWriter, r *http.Request) {
	ccCache, err := cloudfoundry.GetGlobalCCCache()
	if err != nil {
		log.Errorf("Could not retrieve CC cache: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	orgQuotas, err := ccCache.GetOrgQuotas()
	if err != nil {
		log.Errorf("Error getting orgQuotas: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	orgQuotasBytes, err := json.Marshal(orgQuotas)
	if err != nil {
		log.Errorf("Could not process CF orgQuotas: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(orgQuotasBytes) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(orgQuotasBytes)
		return
	}
}
