// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !kubeapiserver

package cloudfoundry

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	cfclient "github.com/cloudfoundry/go-cfclient/v3/client"
	cfconfig "github.com/cloudfoundry/go-cfclient/v3/config"
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

// CFClient implements CCClientI on top of the go-cfclient v3 client
type CFClient struct {
	client *cfclient.Client
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
				customTags = append(customTags, extractTagsFromAppMeta(ccApp.Metadata.Labels)...)
				customTags = append(customTags, extractTagsFromAppMeta(ccApp.Metadata.Annotations)...)
			}

			spaceGUID = appSpaceGUID(ccApp)
			if space, err := ccCache.GetSpace(spaceGUID); err == nil {
				spaceName = space.Name
				orgGUID = spaceOrgGUID(space)
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

// mergeMetadata adds the non-null entries of metadata that are not already set in dst
func mergeMetadata(dst map[string]string, metadata map[string]*string) {
	for key, value := range metadata {
		if value == nil {
			continue
		}
		if _, ok := dst[key]; !ok {
			dst[key] = *value
		}
	}
}

func appSpaceGUID(app *resource.App) string {
	if app.Relationships.Space.Data == nil {
		return ""
	}
	return app.Relationships.Space.Data.GUID
}

func spaceOrgGUID(space *resource.Space) string {
	if space.Relationships == nil || space.Relationships.Organization == nil || space.Relationships.Organization.Data == nil {
		return ""
	}
	return space.Relationships.Organization.Data.GUID
}

func (a *CFApplication) extractDataFromV3App(data *resource.App) {
	a.GUID = data.GUID
	a.Name = data.Name
	a.SpaceGUID = appSpaceGUID(data)
	switch lifecycle := data.Lifecycle.Data.(type) {
	case *resource.BuildpackLifecycle:
		a.Buildpacks = lifecycle.Buildpacks
	case *resource.CNBLifecycle:
		a.Buildpacks = lifecycle.Buildpacks
	}
	a.Labels = map[string]string{}
	a.Annotations = map[string]string{}
	if data.Metadata != nil {
		mergeMetadata(a.Labels, data.Metadata.Labels)
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
	}
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
	a.OrgGUID = spaceOrgGUID(data)

	// Set space labels and annotations only if they're not overridden per application
	if data.Metadata != nil {
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
		mergeMetadata(a.Labels, data.Metadata.Labels)
	}
}

func (a *CFApplication) extractDataFromV3Org(data *resource.Organization) {
	a.OrgName = data.Name

	// Set org labels and annotations only if they're not overridden per space or application
	if data.Metadata != nil {
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
		mergeMetadata(a.Labels, data.Metadata.Labels)
	}
}

// ccRequestTimeout bounds each Cloud Controller request, page reads included. It matches the gorouter's default
// backend timeout (router.request_timeout_in_seconds): go-cfclient's 30s default fails large pages (up to
// cloud_foundry_cc.apps_batch_size items) that the router, and the previous unbounded client, allow.
const ccRequestTimeout = 900 * time.Second

// NewCFClient returns a new Cloud Foundry client instance. It discovers the UAA endpoint from the v3 API root.
func NewCFClient(apiAddress, clientID, clientSecret string, skipSSLValidation bool) (*CFClient, error) {
	options := []cfconfig.Option{
		cfconfig.ClientCredentials(clientID, clientSecret),
		// identifies our requests in Cloud Controller access logs
		cfconfig.UserAgent("datadog-cluster-agent"),
		cfconfig.RequestTimeout(ccRequestTimeout),
	}
	if skipSSLValidation {
		options = append(options, cfconfig.SkipTLSValidation())
	}
	cfg, err := cfconfig.New(apiAddress, options...)
	if err != nil {
		return nil, err
	}
	cfc, err := cfclient.New(cfg)
	if err != nil {
		return nil, err
	}
	return &CFClient{client: cfc}, nil
}

func listOptions(perPage int) *cfclient.ListOptions {
	return &cfclient.ListOptions{Page: 1, PerPage: perPage}
}

// ListApps returns all applications
func (c *CFClient) ListApps(perPage int) ([]*resource.App, error) {
	return c.client.Applications.ListAll(context.Background(), &cfclient.AppListOptions{ListOptions: listOptions(perPage)})
}

// ListOrganizations returns all organizations
func (c *CFClient) ListOrganizations(perPage int) ([]*resource.Organization, error) {
	return c.client.Organizations.ListAll(context.Background(), &cfclient.OrganizationListOptions{ListOptions: listOptions(perPage)})
}

// ListSpaces returns all spaces
func (c *CFClient) ListSpaces(perPage int) ([]*resource.Space, error) {
	return c.client.Spaces.ListAll(context.Background(), &cfclient.SpaceListOptions{ListOptions: listOptions(perPage)})
}

// ListProcesses returns all processes
func (c *CFClient) ListProcesses(perPage int) ([]*resource.Process, error) {
	return c.client.Processes.ListAll(context.Background(), &cfclient.ProcessListOptions{ListOptions: listOptions(perPage)})
}

// ListOrgQuotas returns all organization quotas
func (c *CFClient) ListOrgQuotas(perPage int) ([]*resource.OrganizationQuota, error) {
	return c.client.OrganizationQuotas.ListAll(context.Background(), &cfclient.OrganizationQuotaListOptions{ListOptions: listOptions(perPage)})
}

// ListIsolationSegments returns all isolation segments
func (c *CFClient) ListIsolationSegments(perPage int) ([]*resource.IsolationSegment, error) {
	return c.client.IsolationSegments.ListAll(context.Background(), &cfclient.IsolationSegmentListOptions{ListOptions: listOptions(perPage)})
}

// ListSidecarsByApp returns a list of sidecars for the given application GUID
func (c *CFClient) ListSidecarsByApp(appGUID string, perPage int) ([]CFSidecar, error) {
	sidecars, err := c.client.Sidecars.ListForAppAll(context.Background(), appGUID, &cfclient.SidecarListOptions{ListOptions: listOptions(perPage)})
	if err != nil {
		return nil, fmt.Errorf("Error requesting sidecars for app %s: %w", appGUID, err)
	}
	res := make([]CFSidecar, 0, len(sidecars))
	for _, sidecar := range sidecars {
		res = append(res, CFSidecar{Name: sidecar.Name, GUID: sidecar.GUID})
	}
	return res, nil
}

// GetIsolationSegmentSpaceGUID returns the GUID of the first space the given isolation segment is entitled to
func (c *CFClient) GetIsolationSegmentSpaceGUID(guid string) (string, error) {
	spaceGUIDs, err := c.client.IsolationSegments.ListSpaceRelationships(context.Background(), guid)
	return firstGUID(spaceGUIDs, err)
}

// GetIsolationSegmentOrganizationGUID returns the GUID of the first organization the given isolation segment is entitled to
func (c *CFClient) GetIsolationSegmentOrganizationGUID(guid string) (string, error) {
	orgGUIDs, err := c.client.IsolationSegments.ListOrganizationRelationships(context.Background(), guid)
	return firstGUID(orgGUIDs, err)
}

func firstGUID(guids []string, err error) (string, error) {
	if err != nil || len(guids) == 0 {
		return "", err
	}
	return guids[0], nil
}

// GetApp returns the application with the given GUID
func (c *CFClient) GetApp(guid string) (*resource.App, error) {
	return c.client.Applications.Get(context.Background(), guid)
}

// GetSpace returns the space with the given GUID
func (c *CFClient) GetSpace(guid string) (*resource.Space, error) {
	return c.client.Spaces.Get(context.Background(), guid)
}

// GetOrganization returns the organization with the given GUID
func (c *CFClient) GetOrganization(guid string) (*resource.Organization, error) {
	return c.client.Organizations.Get(context.Background(), guid)
}

// ListProcessesByApp returns a list of processes for the given application GUID
func (c *CFClient) ListProcessesByApp(appGUID string, perPage int) ([]*resource.Process, error) {
	return c.client.Processes.ListForAppAll(context.Background(), appGUID, &cfclient.ProcessListOptions{ListOptions: listOptions(perPage)})
}
