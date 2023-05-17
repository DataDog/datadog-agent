// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package cloudfoundry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudfoundry-community/go-cfclient"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"code.cloudfoundry.org/bbs/models"
)

const (
	// EnvAdVariableName is the name of the environment variable storing AD settings
	EnvAdVariableName = "AD_DATADOGHQ_COM"
	// EnvVcapServicesVariableName is the name of the environment variable storing the services for the app
	EnvVcapServicesVariableName = "VCAP_SERVICES"
	// EnvVcapApplicationName is the name of the environment variable storing the application definition
	EnvVcapApplicationName = "VCAP_APPLICATION"
	// ActualLrpStateRunning is the value for the running state o LRP
	ActualLrpStateRunning = "RUNNING"
	// ApplicationNameKey is the name of the key containing the app name in the env var VCAP_APPLICATION
	ApplicationNameKey = "application_name"
	// ApplicationIDKey is the name of the key containing the app GUID in the env var VCAP_APPLICATION
	ApplicationIDKey = "application_id"
	// SpaceNameKey is the name of the key containing the space name in the env var VCAP_APPLICATION
	SpaceNameKey = "space_name"
	// SpaceIDKey is the name of the key containing the space GUID in the env var VCAP_APPLICATION
	SpaceIDKey = "space_id"
	// OrganizationNameKey is the name of the key containing the organization name in the env var VCAP_APPLICATION
	OrganizationNameKey = "organization_name"
	// OrganizationIDKey is the name of the key containing the organization GUID in the env var VCAP_APPLICATION
	OrganizationIDKey = "organization_id"
	// AutodiscoveryTagsMetaPrefix is the prefix of labels/annotations to look for to tag containers
	AutodiscoveryTagsMetaPrefix = "tags.datadoghq.com/"
)

var (
	envVcapApplicationKeys = []string{
		ApplicationNameKey, ApplicationIDKey, OrganizationNameKey, OrganizationIDKey, SpaceNameKey, SpaceIDKey,
	}
)

// ADConfig represents the structure of ADConfig in AD_DATADOGHQ_COM environment variable
// the AD config looks like:
// {"my-http-app": {"check_names": ..., "init_configs": ..., "instances": ..., "variables": ...}, ...}
// we need to unmarshal the values of check_names, init_configs and instances to json.RawMessage
// to be able to pass them to extractTemplatesFromMap
type ADConfig map[string]map[string]json.RawMessage

// ADIdentifier is a structure that carries all data necessary for creating an AD Identifier for Cloud Foundry service
type ADIdentifier struct {
	desiredLRP DesiredLRP
	svcName    string
	actualLRP  *ActualLRP
}

// NewADNonContainerIdentifier creates a new ADIdentifier for a service not running in a container
func NewADNonContainerIdentifier(desiredLRP DesiredLRP, svcName string) ADIdentifier {
	return ADIdentifier{
		desiredLRP: desiredLRP,
		svcName:    svcName,
	}
}

// NewADContainerIdentifier creates a new ADIdentifier for a service running in a container
func NewADContainerIdentifier(desiredLRP DesiredLRP, svcName string, actualLRP ActualLRP) ADIdentifier {
	id := NewADNonContainerIdentifier(desiredLRP, svcName)
	id.actualLRP = &actualLRP
	return id
}

// GetActualLRP returns ActualLRP that is part of this ADIdentifier (null for non-container services)
func (id ADIdentifier) GetActualLRP() *ActualLRP {
	return id.actualLRP
}

// GetDesiredLRP returns DesiredLRP that is part of this ADIdentifier
func (id ADIdentifier) GetDesiredLRP() *DesiredLRP {
	return &(id.desiredLRP)
}

// String returns the string representation of this ADIdentifier
func (id ADIdentifier) String() string {
	if id.actualLRP != nil {
		// For container checks, use processGUID to have 1 check per container, even during rolling redeployments
		return fmt.Sprintf("%s/%s/%s", id.desiredLRP.ProcessGUID, id.svcName, id.actualLRP.InstanceGUID)
	}
	// For non container checks, use appGUID to have one check per service, even during rolling redeployments
	return fmt.Sprintf("%s/%s", id.desiredLRP.AppGUID, id.svcName)
}

// ActualLRP carries the necessary data about an Actual LRP obtained through BBS API
type ActualLRP struct {
	CellID       string
	ContainerIP  string
	Index        int32
	Ports        []uint32
	ProcessGUID  string
	InstanceGUID string
	State        string
}

// DesiredLRP carries the necessary data about a Desired LRP obtained through BBS API
type DesiredLRP struct {
	AppGUID            string
	AppName            string
	EnvAD              ADConfig
	EnvVcapServices    map[string][]byte
	EnvVcapApplication map[string]string
	OrganizationGUID   string
	OrganizationName   string
	ProcessGUID        string
	SpaceGUID          string
	SpaceName          string
	CustomTags         []string
}

// CFClient defines a structure that implements the official go cf-client and implements methods that are not supported yet
type CFClient struct {
	*cfclient.Client
}

// CFApplication represents a Cloud Foundry application regardless of the CAPI version
type CFApplication struct {
	GUID           string
	Name           string
	SpaceGUID      string
	SpaceName      string
	OrgName        string
	OrgGUID        string
	Instances      int
	Buildpacks     []string
	DiskQuota      int
	TotalDiskQuota int
	Memory         int
	TotalMemory    int
	Labels         map[string]string
	Annotations    map[string]string
	Sidecars       []CFSidecar
}

// CFSidecar defines a Cloud Foundry Sidecar
type CFSidecar struct {
	Name string
	GUID string
}

type listSidecarsResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources  []CFSidecar         `json:"resources"`
}

type isolationSegmentRelationshipResponse struct {
	Data []struct {
		GUID string `json:"guid"`
	} `json:"data"`
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		Related struct {
			Href string `json:"href"`
		} `json:"related"`
	} `json:"links"`
}

type listProcessesByAppGUIDResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources  []cfclient.Process  `json:"resources"`
}

// CFOrgQuota defines a Cloud Foundry Organization quota
type CFOrgQuota struct {
	GUID        string
	MemoryLimit int
}

// ActualLRPFromBBSModel creates a new ActualLRP from BBS's ActualLRP model
func ActualLRPFromBBSModel(bbsLRP *models.ActualLRP) ActualLRP {
	ports := []uint32{}
	for _, pm := range bbsLRP.Ports {
		ports = append(ports, pm.ContainerPort)
	}
	a := ActualLRP{
		CellID:       bbsLRP.CellId,
		ContainerIP:  bbsLRP.InstanceAddress,
		Index:        bbsLRP.Index,
		Ports:        ports,
		ProcessGUID:  bbsLRP.ProcessGuid,
		State:        bbsLRP.State,
		InstanceGUID: bbsLRP.InstanceGuid,
	}
	return a
}

// DesiredLRPFromBBSModel creates a new DesiredLRP from BBS's DesiredLRP model
func DesiredLRPFromBBSModel(bbsLRP *models.DesiredLRP, includeList, excludeList []*regexp.Regexp) DesiredLRP {
	envAD := ADConfig{}
	envVS := map[string][]byte{}
	envVA := map[string]string{}
	actionEnvs := [][]*models.EnvironmentVariable{}
	// Actions are a nested structure, e.g parallel action might contain two serial actions etc
	// We go through all actions breadth-first and record environment from all run actions,
	// since these are the ones we need to find
	actionQueue := []*models.Action{bbsLRP.Action}
	for len(actionQueue) > 0 {
		action := actionQueue[0]
		actionQueue = actionQueue[1:]

		if a := action.GetRunAction(); a != nil {
			actionEnvs = append(actionEnvs, a.Env)
		} else if a := action.GetTimeoutAction(); a != nil {
			actionQueue = append(actionQueue, a.Action)
		} else if a := action.GetEmitProgressAction(); a != nil {
			actionQueue = append(actionQueue, a.Action)
		} else if a := action.GetTryAction(); a != nil {
			actionQueue = append(actionQueue, a.Action)
		} else if a := action.GetParallelAction(); a != nil {
			actionQueue = append(actionQueue, a.Actions...)
		} else if a := action.GetSerialAction(); a != nil {
			actionQueue = append(actionQueue, a.Actions...)
		} else if a := action.GetCodependentAction(); a != nil {
			actionQueue = append(actionQueue, a.Actions...)
		}
	}

	var customTags []string
	var err error
	for _, envVars := range actionEnvs {
		for _, ev := range envVars {
			if ev.Name == EnvAdVariableName {
				err = json.Unmarshal([]byte(ev.Value), &envAD)
				if err != nil {
					_ = log.Errorf("Failed unmarshalling %s env variable for LRP %s: %s",
						EnvAdVariableName, bbsLRP.ProcessGuid, err.Error())
				}
			} else if ev.Name == EnvVcapServicesVariableName {
				envVS, err = getVcapServicesMap(ev.Value, bbsLRP.ProcessGuid)
				if err != nil {
					_ = log.Errorf("Failed unmarshalling %s env variable for LRP %s: %s",
						EnvVcapServicesVariableName, bbsLRP.ProcessGuid, err.Error())
				}
			} else if ev.Name == EnvVcapApplicationName {
				envVA, err = getVcapApplicationMap(ev.Value)
				if err != nil {
					_ = log.Errorf("Failed unmarshalling %s env variable for LRP %s: %s",
						EnvVcapApplicationName, bbsLRP.ProcessGuid, err.Error())
				}
			}

			if isAllowedTag(ev.Name, includeList, excludeList) {
				customTags = append(customTags, fmt.Sprintf("%s:%s", ev.Name, ev.Value))
			}
		}
		if len(envAD) > 0 {
			// NOTE: it seems there can't be more different AD env variables in all actions
			break
		}
	}
	extractVA := map[string]string{
		ApplicationIDKey:    "",
		ApplicationNameKey:  "",
		OrganizationIDKey:   "",
		OrganizationNameKey: "",
		SpaceIDKey:          "",
		SpaceNameKey:        "",
	}
	for key := range extractVA {
		var ok bool
		extractVA[key], ok = envVA[key]
		if !ok || extractVA[key] == "" {
			log.Tracef("Couldn't extract %s from LRP %s", key, bbsLRP.ProcessGuid)
		}
	}
	appName := extractVA[ApplicationNameKey]
	appGUID := extractVA[ApplicationIDKey]
	orgGUID := extractVA[OrganizationIDKey]
	orgName := extractVA[OrganizationNameKey]
	spaceGUID := extractVA[SpaceIDKey]
	spaceName := extractVA[SpaceNameKey]
	// try to get updated app name from CC API in case of app renames, as well as tags extracted from app metadata
	ccCache, err := GetGlobalCCCache()
	if err == nil {
		if ccApp, err := ccCache.GetApp(appGUID); err != nil {
			log.Debugf("Could not find app %s in cc cache", appGUID)
		} else {
			appName = ccApp.Name

			tags := extractTagsFromAppMeta(ccApp.Metadata.Labels)
			tags = append(tags, extractTagsFromAppMeta(ccApp.Metadata.Annotations)...)
			customTags = append(customTags, tags...)

			spaceGUID = ccApp.Relationships["space"].Data.GUID
			if space, err := ccCache.GetSpace(spaceGUID); err == nil {
				spaceName = space.Name
				orgGUID = space.Relationships["organization"].Data.GUID
			} else {
				log.Debugf("Could not find space %s in cc cache", spaceGUID)
			}
			if org, err := ccCache.GetOrg(orgGUID); err == nil {
				orgName = org.Name
			} else {
				log.Debugf("Could not find org %s in cc cache", orgGUID)
			}
			if ccCache.sidecarsTags {
				if sidecars, err := ccCache.GetSidecars(appGUID); err == nil && len(sidecars) > 0 {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SidecarPresentTagKey, "true"))
					customTags = append(customTags, fmt.Sprintf("%s:%d", SidecarCountTagKey, len(sidecars)))
				} else {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SidecarPresentTagKey, "false"))
				}
			}
			if ccCache.segmentsTags {
				if segment, err := ccCache.GetIsolationSegmentForOrg(orgGUID); err == nil {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentIDTagKey, segment.GUID))
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentNameTagKey, segment.Name))
				} else if segment, err := ccCache.GetIsolationSegmentForSpace(spaceGUID); err == nil {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentIDTagKey, segment.GUID))
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentNameTagKey, segment.Name))
				}
			}
		}
	} else {
		log.Debugf("Could not get Cloud Foundry CCAPI cache: %v", err)
	}

	d := DesiredLRP{
		AppGUID:            appGUID,
		AppName:            appName,
		EnvAD:              envAD,
		EnvVcapServices:    envVS,
		EnvVcapApplication: envVA,
		OrganizationGUID:   orgGUID,
		OrganizationName:   orgName,
		ProcessGUID:        bbsLRP.ProcessGuid,
		SpaceGUID:          spaceGUID,
		SpaceName:          spaceName,
		CustomTags:         customTags,
	}
	return d
}

// GetTagsFromDLRP returns a set of tags extracted from DLRP - names and guids for app, space and org
func (dlrp *DesiredLRP) GetTagsFromDLRP() []string {
	tagsToValues := map[string]string{
		AppNameTagKey: dlrp.AppName,
		AppIDTagKey:   dlrp.AppGUID,
		AppGUIDTagKey: dlrp.AppGUID,
		OrgNameTagKey: dlrp.OrganizationName,
		OrgIDTagKey:   dlrp.OrganizationGUID,
		SpaceNameKey:  dlrp.SpaceName,
		SpaceIDTagKey: dlrp.SpaceGUID,
	}
	tags := []string{}
	for k, v := range tagsToValues {
		if v != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", k, v))
		}
	}
	tags = append(tags, dlrp.CustomTags...)
	sort.Strings(tags)
	return tags
}

func isAllowedTag(value string, includeList, excludeList []*regexp.Regexp) bool {
	// Return false if a key is in excluded
	// Return true if a key is included or there are no excludeList nor includeList patterns
	// excludeList takes precedence, i.e. return false if a key matches both a includeList and excludeList pattern

	// If there is no includeList nor excludeList, return false
	if len(includeList) == 0 && len(excludeList) == 0 {
		return false
	}

	// If there is no includeList, assume at first the value is allowed, then refine decision based on excludeList.
	allowed := len(includeList) == 0

	for _, re := range includeList {
		if re.Match([]byte(value)) {
			allowed = true
			break
		}
	}
	for _, re := range excludeList {
		if re.Match([]byte(value)) {
			allowed = false
			break
		}
	}
	return allowed
}

func getVcapServicesMap(vcap, processGUID string) (map[string][]byte, error) {
	// VCAP_SERVICES maps broker names to lists of service instances
	// e.g. {"broker": [{"name": "my-service-1", ...}, ...], ...}
	var vcMap map[string][]map[string]interface{}
	// we transform the above to {"name": {...}, ...} for easy access
	ret := map[string][]byte{}
	if vcap == "" {
		return ret, nil
	}

	err := json.Unmarshal([]byte(vcap), &vcMap)
	if err != nil {
		return ret, err
	}

	for _, instList := range vcMap {
		for _, inst := range instList {
			if name, ok := inst["name"]; ok {
				nameStr, success := name.(string)
				if !success {
					_ = log.Errorf("Failed converting name of instance %v of LRP %s to string", name, processGUID)
					continue
				}
				serializedInst, err := json.Marshal(inst)
				if err != nil {
					_ = log.Errorf("Failed serializing instance %s of LRP %s to JSON", nameStr, processGUID)
					continue
				}
				ret[nameStr] = serializedInst
			}
		}
	}

	return ret, nil
}

func getVcapApplicationMap(vcap string) (map[string]string, error) {
	// VCAP_APPLICATION describes the application
	// e.g. {"application_id": "...", "application_name": "...", ...
	var vcMap map[string]interface{}
	var res = make(map[string]string)
	if vcap == "" {
		return res, nil
	}

	err := json.Unmarshal([]byte(vcap), &vcMap)
	if err != nil {
		return res, err
	}

	// Keep only needed keys, if they are present
	for _, key := range envVcapApplicationKeys {
		val, ok := vcMap[key]
		if !ok {
			log.Tracef("Could not find key %s in VCAP_APPLICATION env var", key)
			continue
		}
		valString, ok := val.(string)
		if !ok {
			log.Debugf("Could not parse the value of %s as a string", key)
			continue
		}
		res[key] = valString
	}

	return res, nil
}

func extractTagsFromAppMeta(meta map[string]string) (tags []string) {
	for k, v := range meta {
		if strings.HasPrefix(k, AutodiscoveryTagsMetaPrefix) {
			tags = append(tags, fmt.Sprintf("%s:%s", strings.TrimPrefix(k, AutodiscoveryTagsMetaPrefix), v))
		}
	}
	return
}

func (a *CFApplication) extractDataFromV3App(data cfclient.V3App) {
	a.GUID = data.GUID
	a.Name = data.Name
	a.SpaceGUID = data.Relationships["space"].Data.GUID
	a.Buildpacks = data.Lifecycle.BuildpackData.Buildpacks
	a.Annotations = data.Metadata.Annotations
	a.Labels = data.Metadata.Labels
	if a.Annotations == nil {
		a.Annotations = map[string]string{}
	}
	if a.Labels == nil {
		a.Labels = map[string]string{}
	}
}

func (a *CFApplication) extractDataFromV3Process(data []*cfclient.Process) {
	if len(data) <= 0 {
		return
	}
	totalInstances := 0
	totalDiskInMbConfigured := 0
	totalDiskInMbProvisioned := 0
	totalMemoryInMbConfigured := 0
	totalMemoryInMbProvisioned := 0

	for _, p := range data {
		instances := p.Instances
		diskInMbConfigured := p.DiskInMB
		diskInMbProvisioned := instances * diskInMbConfigured
		memoryInMbConfigured := p.MemoryInMB
		memoryInMbProvisioned := instances * memoryInMbConfigured

		totalInstances += instances
		totalDiskInMbConfigured += diskInMbConfigured
		totalDiskInMbProvisioned += diskInMbProvisioned
		totalMemoryInMbConfigured += memoryInMbConfigured
		totalMemoryInMbProvisioned += memoryInMbProvisioned
	}

	a.Instances = totalInstances

	a.DiskQuota = totalDiskInMbConfigured
	a.Memory = totalMemoryInMbConfigured
	a.TotalDiskQuota = totalDiskInMbProvisioned
	a.TotalMemory = totalMemoryInMbProvisioned
}

func (a *CFApplication) extractDataFromV3Space(data *cfclient.V3Space) {
	a.SpaceName = data.Name
	a.OrgGUID = data.Relationships["organization"].Data.GUID

	// Set space labels and annotations only if they're not overridden per application
	for key, value := range data.Metadata.Annotations {
		if _, ok := a.Annotations[key]; !ok {
			a.Annotations[key] = value
		}
	}
	for key, value := range data.Metadata.Labels {
		if _, ok := a.Labels[key]; !ok {
			a.Labels[key] = value
		}
	}
}

func (a *CFApplication) extractDataFromV3Org(data *cfclient.V3Organization) {
	a.OrgName = data.Name

	// Set org labels and annotations only if they're not overridden per space or application
	for key, value := range data.Metadata.Annotations {
		if _, ok := a.Annotations[key]; !ok {
			a.Annotations[key] = value
		}
	}
	for key, value := range data.Metadata.Labels {
		if _, ok := a.Labels[key]; !ok {
			a.Labels[key] = value
		}
	}
}

// NewCFClient returns a new Cloud Foundry client instance given a config
func NewCFClient(config *cfclient.Config) (client *CFClient, err error) {
	cfc, err := cfclient.NewClient(config)
	if err != nil {
		return nil, err
	}
	client = &CFClient{cfc}
	return client, nil
}

// ListSidecarsByApp returns a list of sidecars for the given application GUID
func (c *CFClient) ListSidecarsByApp(query url.Values, appGUID string) ([]CFSidecar, error) {
	var sidecars []CFSidecar

	requestURL := "/v3/apps/" + appGUID + "/sidecars"
	for page := 1; ; page++ {
		query.Set("page", strconv.Itoa(page))
		r := c.NewRequest("GET", requestURL+"?"+query.Encode())
		resp, err := c.DoRequest(r)
		if err != nil {
			return nil, fmt.Errorf("Error requesting sidecars for app: %s", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Error listing sidecars, response code: %d", resp.StatusCode)
		}

		defer resp.Body.Close()
		resBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error reading sidecars response for app %s for page %d: %s", appGUID, page, err)
		}

		var data listSidecarsResponse
		err = json.Unmarshal(resBody, &data)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling sidecars response for app %s for page %d: %s", appGUID, page, err)
		}

		sidecars = append(sidecars, data.Resources...)

		if data.Pagination.TotalPages <= page {
			break
		}
	}
	return sidecars, nil
}

func (c *CFClient) getIsolationSegmentRelationship(resource, guid string) (string, error) {
	requestURL := "/v3/isolation_segments/" + guid + "/relationships/" + resource
	r := c.NewRequest("GET", requestURL)

	resp, err := c.DoRequest(r)
	if err != nil {
		return "", fmt.Errorf("Error requesting isolation segment %s: %s", resource, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Error listing isolation segment %s, response code: %d", resource, resp.StatusCode)
	}

	defer resp.Body.Close()
	resBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Error reading isolation segment %s response: %s", resource, err)
	}

	var data isolationSegmentRelationshipResponse
	err = json.Unmarshal(resBody, &data)
	if err != nil {
		return "", fmt.Errorf("Error unmarshalling isolation segment %s response: %s", resource, err)
	}

	if len(data.Data) == 0 {
		return "", nil
	}

	return data.Data[0].GUID, nil
}

// GetIsolationSegmentSpaceGUID returns an isolation segment GUID given a space GUID
func (c *CFClient) GetIsolationSegmentSpaceGUID(guid string) (string, error) {
	return c.getIsolationSegmentRelationship("spaces", guid)
}

// GetIsolationSegmentOrganizationGUID return an isolation segment GUID given an organization GUID
func (c *CFClient) GetIsolationSegmentOrganizationGUID(guid string) (string, error) {
	return c.getIsolationSegmentRelationship("organizations", guid)
}

// ListProcessByAppGUID returns a list of processes for the given application GUID
func (c *CFClient) ListProcessByAppGUID(query url.Values, appGUID string) ([]cfclient.Process, error) {
	var processes []cfclient.Process

	requestURL := "/v3/apps/" + appGUID + "/processes"
	for page := 1; ; page++ {
		query.Set("page", strconv.Itoa(page))
		r := c.NewRequest("GET", requestURL+"?"+query.Encode())
		resp, err := c.DoRequest(r)
		if err != nil {
			return nil, fmt.Errorf("Error requesting processes for app: %s", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Error listing processes, response code: %d", resp.StatusCode)
		}

		defer resp.Body.Close()
		resBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error reading processes response for app %s for page %d: %s", appGUID, page, err)
		}

		var data listProcessesByAppGUIDResponse
		err = json.Unmarshal(resBody, &data)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling processes response for app %s for page %d: %s", appGUID, page, err)
		}

		processes = append(processes, data.Resources...)

		if data.Pagination.TotalPages <= page {
			break
		}
	}
	return processes, nil
}
