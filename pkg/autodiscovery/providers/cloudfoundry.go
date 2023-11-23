// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
//go:build clusterchecks

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/bhmj/jsonslice"
)

// CloudFoundryConfigProvider implements the Config Provider interface, it should
// be called periodically and returns templates from Cloud Foundry BBS for AutoConf.
type CloudFoundryConfigProvider struct {
	bbsCache      cloudfoundry.BBSCacheI
	lastCollected time.Time
}

// NewCloudFoundryConfigProvider instantiates a new CloudFoundryConfigProvider from given config
func NewCloudFoundryConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	cfp := CloudFoundryConfigProvider{
		lastCollected: time.Now(),
	}
	var err error

	if cfp.bbsCache, err = cloudfoundry.GetGlobalBBSCache(); err != nil {
		return nil, err
	}
	return cfp, nil
}

// String returns a string representation of the CloudFoundryConfigProvider
func (cf CloudFoundryConfigProvider) String() string {
	return names.CloudFoundryBBS
}

// IsUpToDate returns true if the last collection time was later than last BBS Cache refresh time
func (cf CloudFoundryConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return cf.lastCollected.After(cf.bbsCache.LastUpdated()), nil
}

// Collect collects AD config templates from all relevant BBS API information
func (cf CloudFoundryConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Debug("Collecting configs via the CloudFoundryProvider")
	cf.lastCollected = time.Now()
	allActualLRPs, desiredLRPs := cf.bbsCache.GetAllLRPs()
	allConfigs := []integration.Config{}
	for _, desiredLRP := range desiredLRPs {
		actualLRPs, ok := allActualLRPs[desiredLRP.ProcessGUID]
		if !ok {
			actualLRPs = []*cloudfoundry.ActualLRP{}
		}
		newConfigs := cf.getConfigsForApp(desiredLRP, actualLRPs)
		log.Debugf("Successfully got %d configs for app %s", len(newConfigs), desiredLRP.AppGUID)
		allConfigs = append(allConfigs, newConfigs...)
	}
	return allConfigs, nil
}

func (cf CloudFoundryConfigProvider) getConfigsForApp(desiredLRP *cloudfoundry.DesiredLRP, actualLRPs []*cloudfoundry.ActualLRP) []integration.Config {
	allConfigs := []integration.Config{}

	for adName, adVal := range desiredLRP.EnvAD {
		// initially, let's assume a non-container service; we'll change to container service in
		// `expandPerContainerChecks` if necessary
		id := cloudfoundry.NewADNonContainerIdentifier(*desiredLRP, adName)
		// we need to convert adVal to map[string]string to pass it to extractTemplatesFromMap
		convertedADVal := map[string]string{}
		for k, v := range adVal {
			convertedADVal[k] = string(v)
		}
		parsedConfigs, errs := utils.ExtractTemplatesFromMap(id.String(), convertedADVal, "")
		for _, err := range errs {
			log.Errorf("Cannot parse endpoint template for service %s of app %s: %s, skipping",
				adName, desiredLRP.AppGUID, err)
		}

		vcVal, vcOk := desiredLRP.EnvVcapServices[adName]
		variables, varsOk := adVal["variables"]
		success := false
		switch {
		case vcOk:
			// if service is found in VCAP_SERVICES (non-container service), we will run a single check per App
			err := cf.renderExtractedConfigs(parsedConfigs, variables, vcVal)
			if err != nil {
				log.Errorf("Failed to render config for service %s of app %s: %s", adName, desiredLRP.AppGUID, err)
			} else {
				success = true
			}
			cf.assignNodeNameToNonContainerChecks(parsedConfigs, desiredLRP, actualLRPs)
		case varsOk:
			allSvcs := make([]string, 0, len(desiredLRP.EnvVcapServices))
			for s := range desiredLRP.EnvVcapServices {
				allSvcs = append(allSvcs, s)
			}
			allSvcsStr := strings.Join(allSvcs, ", ")
			if allSvcsStr == "" {
				allSvcsStr = "no services found"
			}
			log.Errorf(
				"Service %s for app %s has variables configured, but is not present in VCAP_SERVICES (found services: %s)",
				adName, desiredLRP.AppGUID, allSvcsStr,
			)
		default:
			// if a service is not in VCAP_SERVICES and has no "variables" configured, we want to run a check per container
			parsedConfigs = cf.expandPerContainerChecks(parsedConfigs, desiredLRP, actualLRPs, adName)
			success = true
		}

		if success {
			// mark all checks as cluster checks
			for i := range parsedConfigs {
				parsedConfigs[i].ClusterCheck = true
				parsedConfigs[i].ServiceID = parsedConfigs[i].ADIdentifiers[0]
			}
			allConfigs = append(allConfigs, parsedConfigs...)
		}
	}

	return allConfigs
}

func (cf CloudFoundryConfigProvider) assignNodeNameToNonContainerChecks(configs []integration.Config, desiredLRP *cloudfoundry.DesiredLRP, actualLRPs []*cloudfoundry.ActualLRP) {
	if len(actualLRPs) > 0 {
		aLRP := actualLRPs[0]
		log.Debugf("All non-container checks for app %s will run on Cell %s", desiredLRP.AppGUID, aLRP.CellID)
		for i := range configs {
			configs[i].NodeName = aLRP.CellID
		}
	} else {
		log.Infof("No container running for app %s, checks for its non-container services will run on arbitrary node", desiredLRP.AppGUID)
	}
}

func (cf CloudFoundryConfigProvider) expandPerContainerChecks(
	configs []integration.Config, desiredLRP *cloudfoundry.DesiredLRP, actualLRPs []*cloudfoundry.ActualLRP, svcName string) []integration.Config {
	res := []integration.Config{}
	for _, cfg := range configs {
		for _, aLRP := range actualLRPs {
			// we append container index to AD Identifier distinguish configs for different containers
			newCfg := integration.Config{
				ADIdentifiers: []string{cloudfoundry.NewADContainerIdentifier(*desiredLRP, svcName, *aLRP).String()},
				ClusterCheck:  cfg.ClusterCheck,
				InitConfig:    cfg.InitConfig,
				Instances:     cfg.Instances,
				LogsConfig:    cfg.LogsConfig,
				MetricConfig:  cfg.MetricConfig,
				Name:          cfg.Name,
				// make sure this check runs on the node that's running this container
				NodeName: aLRP.CellID,
				Provider: cfg.Provider,
				Source:   cfg.Source,
			}
			res = append(res, newCfg)
		}
	}

	return res
}

func (cf CloudFoundryConfigProvider) renderExtractedConfigs(configs []integration.Config, variables json.RawMessage, vcap []byte) error {
	var vars map[string]string
	err := json.Unmarshal(variables, &vars)
	if err != nil {
		return fmt.Errorf("failed parsing 'variables' section: %s", err.Error())
	}
	replaceList := []string{}
	for varName, varPath := range vars {
		value, err := jsonslice.Get(vcap, varPath)
		if err != nil {
			return fmt.Errorf("failed extracting variable '%s' from VCAP_SERVICES: %s", varName, err.Error())
		}
		valStr := string(value)
		if len(valStr) > 0 {
			// remove all \", [] and {} from results; users can easily add these themselves, but they wouldn't be able
			// to remove them easily
			switch valStr[0] {
			case '"':
				valStr = strings.Trim(string(value), "\"")
			case '[':
				valStr = strings.Trim(string(value), "[]")
			case '{':
				valStr = strings.Trim(string(value), "{}")
			}
		}
		replaceList = append(replaceList, fmt.Sprintf("%%%%%s%%%%", varName), valStr)
	}

	replacer := strings.NewReplacer(replaceList...)

	for _, cfg := range configs {
		for i, inst := range cfg.Instances {
			newInst := replacer.Replace(string(inst))
			cfg.Instances[i] = integration.Data(newInst)
		}
	}

	return nil
}

func init() {
	RegisterProvider(names.CloudFoundryBBS, NewCloudFoundryConfigProvider)
}

// GetConfigErrors is not implemented for the CloudFoundryConfigProvider
func (cf CloudFoundryConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
