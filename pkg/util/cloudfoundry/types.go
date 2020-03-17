// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"encoding/json"
	"fmt"

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
)

var (
	envVcapApplicationKeys = []string{ApplicationNameKey}
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

// String returns the string representation of this ADIdentifier
func (id ADIdentifier) String() string {
	ret := fmt.Sprintf("%s/%s", id.desiredLRP.ProcessGUID, id.svcName)
	if id.actualLRP != nil {
		ret = fmt.Sprintf("%s/%d", ret, id.actualLRP.Index)
	}
	return ret
}

// ActualLRP carries the necessary data about an Actual LRP obtained through BBS API
type ActualLRP struct {
	AppGUID      string
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
	EnvAD              ADConfig
	EnvVcapServices    map[string][]byte
	EnvVcapApplication map[string]string
	ProcessGUID        string
}

// ActualLRPFromBBSModel creates a new ActualLRP from BBS's ActualLRP model
func ActualLRPFromBBSModel(bbsLRP *models.ActualLRP) ActualLRP {
	ports := []uint32{}
	for _, pm := range bbsLRP.Ports {
		ports = append(ports, pm.ContainerPort)
	}
	a := ActualLRP{
		AppGUID:      appGUIDFromProcessGUID(bbsLRP.ProcessGuid),
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
func DesiredLRPFromBBSModel(bbsLRP *models.DesiredLRP) DesiredLRP {
	envAD := ADConfig{}
	envVS := map[string][]byte{}
	envVA := map[string]string{}
	actionEnvs := [][]*models.EnvironmentVariable{}
	appGUID := appGUIDFromProcessGUID(bbsLRP.ProcessGuid)
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

	var err error
	for _, envVars := range actionEnvs {
		for _, ev := range envVars {
			if ev.Name == EnvAdVariableName {
				err = json.Unmarshal([]byte(ev.Value), &envAD)
				if err != nil {
					log.Errorf("Failed unmarshalling %s env variable for app %s: %s",
						EnvAdVariableName, appGUID, err.Error())
				}
			} else if ev.Name == EnvVcapServicesVariableName {
				envVS, err = getVcapServicesMap(ev.Value, appGUID)
				if err != nil {
					log.Errorf("Failed unmarshalling %s env variable for app %s: %s",
						EnvVcapServicesVariableName, appGUID, err.Error())
				}
			} else if ev.Name == EnvVcapApplicationName {
				envVA, err = getVcapApplicationMap(ev.Value)
				if err != nil {
					log.Errorf("Failed unmarshalling %s env variable for app %s: %s",
						EnvVcapApplicationName, appGUID, err.Error())
				}
			}
		}
		if len(envAD) > 0 {
			// NOTE: it seems there can't be more different AD env variables in all actions
			break
		}
	}
	d := DesiredLRP{
		AppGUID:            appGUID,
		EnvAD:              envAD,
		EnvVcapServices:    envVS,
		EnvVcapApplication: envVA,
		ProcessGUID:        bbsLRP.ProcessGuid,
	}
	return d
}

func appGUIDFromProcessGUID(processGUID string) string {
	return processGUID[0:36]
}

func getVcapServicesMap(vcap, appGUID string) (map[string][]byte, error) {
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
					log.Errorf("Failed converting name of instance %v of App %s to string", name, appGUID)
					continue
				}
				serializedInst, err := json.Marshal(inst)
				if err != nil {
					log.Errorf("Failed serializing instance %s of App %s to JSON", nameStr, appGUID)
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
