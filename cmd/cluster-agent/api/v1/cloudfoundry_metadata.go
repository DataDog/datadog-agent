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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CloudFoundryMetadataHandler handles Cloud Foundry metadata HTTP requests
type CloudFoundryMetadataHandler struct {
	bbsCache cloudfoundry.BBSCacheI
	ccCache  cloudfoundry.CCCacheI
}

// NewCloudFoundryMetadataHandler creates a new CloudFoundryMetadataHandler with the given caches
func NewCloudFoundryMetadataHandler(bbsCache cloudfoundry.BBSCacheI, ccCache cloudfoundry.CCCacheI) *CloudFoundryMetadataHandler {
	return &CloudFoundryMetadataHandler{
		bbsCache: bbsCache,
		ccCache:  ccCache,
	}
}

func installCloudFoundryMetadataEndpoints(r *mux.Router) {
	// Get the Cloud Foundry caches for the metadata handlers
	bbsCache, err := cloudfoundry.GetGlobalBBSCache()
	if err != nil {
		log.Debugf("Could not get BBS cache: %v", err)
	}
	ccCache, err := cloudfoundry.GetGlobalCCCache()
	if err != nil {
		log.Debugf("Could not get CC cache: %v", err)
	}

	handler := NewCloudFoundryMetadataHandler(bbsCache, ccCache)

	r.HandleFunc("/tags/cf/apps/{nodeName}", api.WithTelemetryWrapper("getCFAppsMetadataForNode", handler.getCFAppsMetadataForNode)).Methods("GET")

	if pkgconfigsetup.Datadog().GetBool("cluster_agent.serve_nozzle_data") {
		r.HandleFunc("/cf/apps/{guid}", api.WithTelemetryWrapper("getCFApplication", handler.getCFApplication)).Methods("GET")
		r.HandleFunc("/cf/apps", api.WithTelemetryWrapper("getCFApplications", handler.getCFApplications)).Methods("GET")
		r.HandleFunc("/cf/org_quotas", api.WithTelemetryWrapper("getCFOrgQuotas", handler.getCFOrgQuotas)).Methods("GET")
		r.HandleFunc("/cf/orgs", api.WithTelemetryWrapper("getCFOrgs", handler.getCFOrgs)).Methods("GET")
	}
}

func installKubernetesMetadataEndpoints(r *mux.Router, w workloadmeta.Component) {}

// getCFAppsMetadataForNode is only used when the node agent hits the DCA for the list of cloudfoundry applications tags
// It return a list of tags for each application that can be directly used in the tagger
func (h *CloudFoundryMetadataHandler) getCFAppsMetadataForNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nodename := vars["nodeName"]

	if h.bbsCache == nil {
		log.Errorf("BBS cache is not initialized")
		http.Error(w, "BBS cache is not initialized", http.StatusInternalServerError)
		return
	}

	tags, err := h.bbsCache.GetTagsForNode(nodename)
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
func (h *CloudFoundryMetadataHandler) getCFApplications(w http.ResponseWriter, r *http.Request) {
	if h.ccCache == nil {
		log.Errorf("CC cache is not initialized")
		http.Error(w, "CC cache is not initialized", http.StatusInternalServerError)
		return
	}

	apps, err := h.ccCache.GetCFApplications()
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
func (h *CloudFoundryMetadataHandler) getCFApplication(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	guid := vars["guid"]

	if h.ccCache == nil {
		log.Errorf("CC cache is not initialized")
		http.Error(w, "CC cache is not initialized", http.StatusInternalServerError)
		return
	}

	app, err := h.ccCache.GetCFApplication(guid)
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
func (h *CloudFoundryMetadataHandler) getCFOrgs(w http.ResponseWriter, r *http.Request) {
	if h.ccCache == nil {
		log.Errorf("CC cache is not initialized")
		http.Error(w, "CC cache is not initialized", http.StatusInternalServerError)
		return
	}

	orgs, err := h.ccCache.GetOrgs()
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
func (h *CloudFoundryMetadataHandler) getCFOrgQuotas(w http.ResponseWriter, r *http.Request) {
	if h.ccCache == nil {
		log.Errorf("CC cache is not initialized")
		http.Error(w, "CC cache is not initialized", http.StatusInternalServerError)
		return
	}

	orgQuotas, err := h.ccCache.GetOrgQuotas()
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
