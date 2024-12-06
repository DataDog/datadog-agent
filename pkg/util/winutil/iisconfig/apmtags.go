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
}

var (
	errorlogcount = 0
)

// ReadDotNetConfig reads an iis config file(xml) and returns the APM tags
func ReadDotNetConfig(cfgpath string) (APMTags, error) { //(APMTags, error) {
	var newcfg appConfiguration
	var apmtags APMTags
	var chasedatadogJSON string
	f, err := os.ReadFile(cfgpath)
	if err != nil {
		return apmtags, err
	}
	err = xml.Unmarshal(f, &newcfg)
	if err != nil {
		if dotnetConfigLogCount%logErrorCountInterval == 0 {
			log.Warnf("Error reading datadog.json file %s: %v", cfgpath, err)
			jsonLogCount++
		}
		return apmtags, err
	}
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
			if len(ddjson.DDService) > 0 {
				apmtags.DDService = ddjson.DDService
			}
			if len(ddjson.DDEnv) > 0 {
				apmtags.DDEnv = ddjson.DDEnv
			}
			if len(ddjson.DDVersion) > 0 {
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
	return apmtags, nil
}
