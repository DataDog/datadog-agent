// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package cloudfoundry

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"

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
	*client.Client
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
func DesiredLRPFromBBSModel(bbsLRP *models.DesiredLRP, includeList, excludeList []*regexp.Regexp, ccCache CCCacheI) DesiredLRP {
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
	if ccCache == nil {
		log.Debugf("CCCache is nil, skipping app metadata enrichment for LRP %s", bbsLRP.ProcessGuid)
	} else {
		if ccApp, err := ccCache.GetApp(appGUID); err != nil {
			log.Debugf("Could not find app %s in cc cache", appGUID)
		} else {
			appName = ccApp.Name

			if ccApp.Metadata != nil {
				tags := extractTagsFromAppMeta(ccApp.Metadata.Labels)
				tags = append(tags, extractTagsFromAppMeta(ccApp.Metadata.Annotations)...)
				customTags = append(customTags, tags...)
			}

			spaceGUID = extractGUIDFromRelationship(&ccApp.Relationships.Space)
			if space, err := ccCache.GetSpace(spaceGUID); err == nil {
				spaceName = space.Name
				if space.Relationships != nil {
					orgGUID = extractGUIDFromRelationship(space.Relationships.Organization)
				}
			} else {
				log.Debugf("Could not find space %s in cc cache", spaceGUID)
			}
			if org, err := ccCache.GetOrg(orgGUID); err == nil {
				orgName = org.Name
			} else {
				log.Debugf("Could not find org %s in cc cache", orgGUID)
			}
			if ccCache.SidecarsTagsEnabled() {
				if sidecars, err := ccCache.GetSidecars(appGUID); err == nil && len(sidecars) > 0 {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SidecarPresentTagKey, "true"))
					customTags = append(customTags, fmt.Sprintf("%s:%d", SidecarCountTagKey, len(sidecars)))
				} else {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SidecarPresentTagKey, "false"))
				}
			}
			if ccCache.SegmentsTagsEnabled() {
				if segment, err := ccCache.GetIsolationSegmentForOrg(orgGUID); err == nil {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentIDTagKey, segment.GUID))
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentNameTagKey, segment.Name))
				} else if segment, err := ccCache.GetIsolationSegmentForSpace(spaceGUID); err == nil {
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentIDTagKey, segment.GUID))
					customTags = append(customTags, fmt.Sprintf("%s:%s", SegmentNameTagKey, segment.Name))
				}
			}
		}
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

func extractTagsFromAppMeta(meta map[string]*string) (tags []string) {
	for k, v := range meta {
		if v == nil {
			continue
		}
		if after, ok := strings.CutPrefix(k, AutodiscoveryTagsMetaPrefix); ok {
			tags = append(tags, fmt.Sprintf("%s:%s", after, *v))
		}
	}
	return
}

func (a *CFApplication) extractDataFromV3App(data resource.App) {
	a.GUID = data.GUID
	a.Name = data.Name
	a.SpaceGUID = extractGUIDFromRelationship(&data.Relationships.Space)
	if bp, ok := data.Lifecycle.Data.(*resource.BuildpackLifecycle); ok {
		a.Buildpacks = bp.Buildpacks
	}
	a.Annotations = map[string]string{}
	a.Labels = map[string]string{}
	a.mergeAnnotationsAndLabels(data.Metadata)
}

func (a *CFApplication) extractDataFromV3Process(data []*resource.Process) {
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

func (a *CFApplication) extractDataFromV3Space(data *resource.Space) {
	a.SpaceName = data.Name
	if data.Relationships != nil {
		a.OrgGUID = extractGUIDFromRelationship(data.Relationships.Organization)
	}
	// Set space labels and annotations only if they're not overridden per application
	a.mergeAnnotationsAndLabels(data.Metadata)
}

func extractGUIDFromRelationship(rel *resource.ToOneRelationship) string {
	if rel == nil || rel.Data == nil {
		return ""
	}
	return rel.Data.GUID
}

func (a *CFApplication) extractDataFromV3Org(data *resource.Organization) {
	a.OrgName = data.Name
	// Set org labels and annotations only if they're not overridden per space or application
	a.mergeAnnotationsAndLabels(data.Metadata)
}

func (a *CFApplication) mergeAnnotationsAndLabels(metadata *resource.Metadata) {
	if metadata == nil {
		return
	}
	for key, value := range metadata.Annotations {
		if value == nil {
			continue
		}
		if _, ok := a.Annotations[key]; !ok {
			a.Annotations[key] = *value
		}
	}
	for key, value := range metadata.Labels {
		if value == nil {
			continue
		}
		if _, ok := a.Labels[key]; !ok {
			a.Labels[key] = *value
		}
	}
}

// NewCFClient returns a new Cloud Foundry client instance given a config
func NewCFClient(cfg *config.Config) (*CFClient, error) {
	cfc, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	return &CFClient{cfc}, nil
}

// ListV3AppsByQuery returns all apps visible to the client
func (c *CFClient) ListV3AppsByQuery(perPage PerPage) ([]*resource.App, error) {
	opts := client.NewAppListOptions()
	opts.PerPage = int(perPage)
	apps, err := c.Applications.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing apps: %w", err)
	}
	return apps, nil
}

// ListV3OrganizationsByQuery returns all organizations visible to the client
func (c *CFClient) ListV3OrganizationsByQuery(perPage PerPage) ([]*resource.Organization, error) {
	opts := client.NewOrganizationListOptions()
	opts.PerPage = int(perPage)
	orgs, err := c.Organizations.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing organizations: %w", err)
	}
	return orgs, nil
}

// ListV3SpacesByQuery returns all spaces visible to the client
func (c *CFClient) ListV3SpacesByQuery(perPage PerPage) ([]*resource.Space, error) {
	opts := client.NewSpaceListOptions()
	opts.PerPage = int(perPage)
	spaces, err := c.Spaces.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing spaces: %w", err)
	}
	return spaces, nil
}

// ListAllProcessesByQuery returns all processes visible to the client
func (c *CFClient) ListAllProcessesByQuery(perPage PerPage) ([]*resource.Process, error) {
	opts := client.NewProcessOptions()
	opts.PerPage = int(perPage)
	processes, err := c.Processes.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing processes: %w", err)
	}
	return processes, nil
}

// ListOrgQuotasByQuery returns all organization quotas visible to the client
func (c *CFClient) ListOrgQuotasByQuery(perPage PerPage) ([]*resource.OrganizationQuota, error) {
	opts := client.NewOrganizationQuotaListOptions()
	opts.PerPage = int(perPage)
	quotas, err := c.OrganizationQuotas.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing organization quotas: %w", err)
	}
	return quotas, nil
}

// ListSidecarsByApp returns a list of sidecars for the given application GUID
func (c *CFClient) ListSidecarsByApp(perPage PerPage, appGUID string) ([]CFSidecar, error) {
	opts := client.NewSidecarListOptions()
	opts.PerPage = int(perPage)
	resources, err := c.Sidecars.ListForAppAll(context.Background(), appGUID, opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing sidecars for app %s: %w", appGUID, err)
	}

	sidecars := make([]CFSidecar, 0, len(resources))
	for _, sidecar := range resources {
		sidecars = append(sidecars, CFSidecar{Name: sidecar.Name, GUID: sidecar.GUID})
	}
	return sidecars, nil
}

// ListIsolationSegmentsByQuery returns all isolation segments visible to the client
func (c *CFClient) ListIsolationSegmentsByQuery(perPage PerPage) ([]*resource.IsolationSegment, error) {
	opts := client.NewIsolationSegmentOptions()
	opts.PerPage = int(perPage)
	segments, err := c.IsolationSegments.ListAll(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing isolation segments: %w", err)
	}
	return segments, nil
}

// GetIsolationSegmentSpaceGUID returns an isolation segment GUID given a space GUID
func (c *CFClient) GetIsolationSegmentSpaceGUID(guid string) (string, error) {
	spaceGUIDs, err := c.IsolationSegments.ListSpaceRelationships(context.Background(), guid)
	if err != nil {
		return "", fmt.Errorf("Error requesting isolation segment spaces: %w", err)
	}
	if len(spaceGUIDs) == 0 {
		return "", nil
	}
	return spaceGUIDs[0], nil
}

// GetIsolationSegmentOrganizationGUID return an isolation segment GUID given an organization GUID
func (c *CFClient) GetIsolationSegmentOrganizationGUID(guid string) (string, error) {
	orgGUIDs, err := c.IsolationSegments.ListOrganizationRelationships(context.Background(), guid)
	if err != nil {
		return "", fmt.Errorf("Error requesting isolation segment organizations: %w", err)
	}
	if len(orgGUIDs) == 0 {
		return "", nil
	}
	return orgGUIDs[0], nil
}

// GetV3AppByGUID returns the app with the given GUID
func (c *CFClient) GetV3AppByGUID(guid string) (*resource.App, error) {
	return c.Applications.Get(context.Background(), guid)
}

// GetV3SpaceByGUID returns the space with the given GUID
func (c *CFClient) GetV3SpaceByGUID(guid string) (*resource.Space, error) {
	return c.Spaces.Get(context.Background(), guid)
}

// GetV3OrganizationByGUID returns the organization with the given GUID
func (c *CFClient) GetV3OrganizationByGUID(guid string) (*resource.Organization, error) {
	return c.Organizations.Get(context.Background(), guid)
}

// ListProcessByAppGUID returns a list of processes for the given application GUID
func (c *CFClient) ListProcessByAppGUID(perPage PerPage, appGUID string) ([]*resource.Process, error) {
	opts := client.NewProcessOptions()
	opts.PerPage = int(perPage)
	processes, err := c.Processes.ListForAppAll(context.Background(), appGUID, opts)
	if err != nil {
		return nil, fmt.Errorf("Error listing processes for app %s: %w", appGUID, err)
	}
	return processes, nil
}
