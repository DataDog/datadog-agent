// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cloudfoundry-community/go-cfclient"

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
		return fmt.Sprintf("%s/%s/%d", id.desiredLRP.ProcessGUID, id.svcName, id.actualLRP.Index)
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
	TagsFromEnv        []string
}

// CFApp carries the necessary data about a CF App obtained from the CC API
type CFApp struct {
	Name string
}

func CFAppFromV3App(app *cfclient.V3App) *CFApp {
	return &CFApp{
		Name: app.Name,
	}
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

	var tagsFromEnv []string
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
				tagsFromEnv = append(tagsFromEnv, fmt.Sprintf("%s:%s", ev.Name, ev.Value))
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
			_ = log.Errorf("Couldn't extract %s from LRP %s", key, bbsLRP.ProcessGuid)
		}
	}
	appName := extractVA[ApplicationNameKey]
	appGUID := extractVA[ApplicationIDKey]

	// try to get updated app name from CC API in case of app renames
	ccCache, err := GetGlobalCCCache()
	if err == nil {
		if ccApp, err := ccCache.GetApp(appGUID); err != nil {
			log.Debugf("Could not find app %s in cc cache", appGUID)
		} else {
			appName = ccApp.Name
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
		OrganizationGUID:   extractVA[OrganizationIDKey],
		OrganizationName:   extractVA[OrganizationNameKey],
		ProcessGUID:        bbsLRP.ProcessGuid,
		SpaceGUID:          extractVA[SpaceIDKey],
		SpaceName:          extractVA[SpaceNameKey],
		TagsFromEnv:        tagsFromEnv,
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
	tags = append(tags, dlrp.TagsFromEnv...)
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

	// Keep only needed keys
	for _, key := range envVcapApplicationKeys {
		val, ok := vcMap[key]
		if !ok {
			return res, fmt.Errorf("could not find key %s in VCAP_APPLICATION env var", key)
		}
		valString, ok := val.(string)
		if !ok {
			return res, fmt.Errorf("could not parse the value of %s as a string", key)
		}
		res[key] = valString
	}

	return res, nil
}
