// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package iisconfig manages iis configuration
package iisconfig

/*
the file datadog.json can be located anywhere; it is path-relative to a .net application
give the path name, read the json and return it as a map of string/string
*/

import (
	"encoding/json"
	"encoding/xml"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// APMTags holds the APM tags
type APMTags struct {
	DDService string `json:"DD_SERVICE"`
	DDEnv     string `json:"DD_ENV"`
	DDVersion string `json:"DD_VERSION"`
}

// keep a count of errors to avoid flooding the log
var (
	jsonLogCount          = 0
	dotnetConfigLogCount  = 0
	logErrorCountInterval = 500
)

// ReadDatadogJSON reads a datadog.json file and returns the APM tags
func ReadDatadogJSON(datadogJSONPath string) (APMTags, error) {
	var apmtags APMTags

	file, err := os.Open(datadogJSONPath)
	if err != nil {
		return apmtags, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&apmtags)
	if err != nil {
		if jsonLogCount%logErrorCountInterval == 0 {
			log.Warnf("Error reading datadog.json file %s: %v", datadogJSONPath, err)
			jsonLogCount++
		}
		return apmtags, err
	}
	return apmtags, nil
}

type iisAppSetting struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type iisAppSettings struct {
	XMLName xml.Name        `xml:"appSettings"`
	Adds    []iisAppSetting `xml:"add"`
}

type appConfiguration struct {
	XMLName     xml.Name `xml:"configuration"`
	AppSettings iisAppSettings
	// Presence of <system.webServer><aspNetCore> marks an ASP.NET Core app; see
	// ReadDotNetConfig for how its env vars override <appSettings>.
	SystemWebServer iisLocationSystemWebServer `xml:"system.webServer"`
}

var (
	errorlogcount = 0
)

// ReadDotNetConfig reads an IIS web.config and returns its APM tags. The bool is
// true when they came from <aspNetCore> env vars (real env) vs <appSettings>.
func ReadDotNetConfig(cfgpath string) (APMTags, bool, error) {
	var newcfg appConfiguration
	var apmtags APMTags
	var chasedatadogJSON string
	f, err := os.ReadFile(cfgpath)
	if err != nil {
		return apmtags, false, err
	}
	err = xml.Unmarshal(f, &newcfg)
	if err != nil {
		if dotnetConfigLogCount%logErrorCountInterval == 0 {
			log.Warnf("Error reading datadog.json file %s: %v", cfgpath, err)
			jsonLogCount++
		}
		return apmtags, false, err
	}

	// ASP.NET Core app: <aspNetCore> env vars are real process env; the Core
	// tracer ignores <appSettings> (Framework-only), so we do too.
	if newcfg.SystemWebServer.AspNetCore.XMLName.Local != "" {
		return applyEnvVarsOver(APMTags{}, newcfg.SystemWebServer.AspNetCore.EnvVars), true, nil
	}

	// .NET Framework path: <appSettings>, plus the datadog.json a
	// DD_TRACE_CONFIG_FILE appSetting points to.
	for _, setting := range newcfg.AppSettings.Adds {
		switch setting.Key {
		case "DD_SERVICE":
			apmtags.DDService = setting.Value
		case "DD_ENV":
			apmtags.DDEnv = setting.Value
		case "DD_VERSION":
			apmtags.DDVersion = setting.Value
		case "DD_TRACE_CONFIG_FILE":
			chasedatadogJSON = setting.Value
		}
	}
	if len(chasedatadogJSON) > 0 {
		ddjson, err := ReadDatadogJSON(chasedatadogJSON)
		if err == nil {
			// appSettings outranks datadog.json in the tracer, so only fill
			// fields appSettings left unset.
			if len(apmtags.DDService) == 0 {
				apmtags.DDService = ddjson.DDService
			}
			if len(apmtags.DDEnv) == 0 {
				apmtags.DDEnv = ddjson.DDEnv
			}
			if len(apmtags.DDVersion) == 0 {
				apmtags.DDVersion = ddjson.DDVersion
			}
		} else {
			// only log every logErrorCountInterval occurrences because if this is misconfigured, it could flood the log
			if errorlogcount%logErrorCountInterval == 0 {
				log.Warnf("Error reading configured datadog.json file %s: %v", chasedatadogJSON, err)
			}
			errorlogcount++
		}
	}
	return apmtags, false, nil
}
