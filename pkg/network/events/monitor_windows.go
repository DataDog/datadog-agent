// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events handles process events
package events

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iisconfig"
)

func getProcessStartTime(ev *model.Event) time.Time {
	if ev.GetEventType() == model.ExecEventType {
		return ev.GetProcessExecTime()
	}
	// windows does not have a fork event.
	return time.Time{}
}

func makeTagsSlice(already map[string]struct{}, apmtags iisconfig.APMTags) []*intern.Value {
	tags := make([]*intern.Value, 0, 3)
	if _, found := already["DD_SERVICE"]; !found {
		if len(apmtags.DDService) > 0 {
			tags = append(tags, intern.GetByString("service"+":"+apmtags.DDService))
			already["DD_SERVICE"] = struct{}{}
		}
	}
	if _, found := already["DD_ENV"]; !found {
		if len(apmtags.DDEnv) > 0 {
			tags = append(tags, intern.GetByString("env"+":"+apmtags.DDEnv))
			already["DD_ENV"] = struct{}{}
		}
	}
	if _, found := already["DD_VERSION"]; !found {
		if len(apmtags.DDVersion) > 0 {
			tags = append(tags, intern.GetByString("version"+":"+apmtags.DDVersion))
			already["DD_VERSION"] = struct{}{}
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func getAPMTags(already map[string]struct{}, filename string) []*intern.Value {

	dir := filepath.Dir(filename)
	if dir == "" {
		return nil
	}

	tags := make([]*intern.Value, 0, 3)
	// see if there's an app.config in the directory
	appConfig := filepath.Join(dir, "app.config")
	ddJSON := filepath.Join(dir, "datadog.json")
	if _, err := os.Stat(appConfig); err == nil {

		appcfg, err := iisconfig.ReadDotNetConfig(appConfig)
		if err == nil {
			found := makeTagsSlice(already, appcfg)
			if len(found) > 0 {
				tags = append(tags, found...)
			}
		}
	} else if  !errors.Is(err, os.ErrNotExist) {
		log.Warnf("Error reading app.config: %v", err)
	}
	if len(already) == len(envFilter) {
		// we've seen all we need, no point in looking in datadog.json
		return tags
	}
	// see if there's a datadog.json
	if _, err := os.Stat(ddJSON); err == nil {

		appcfg, err := iisconfig.ReadDatadogJSON(ddJSON)
		if err == nil {
			found := makeTagsSlice(already, appcfg)
			if len(found) > 0 {
				tags = append(tags, found...)
			}
		}
	} else if  !errors.Is(err, os.ErrNotExist) {
		log.Warnf("Error reading app.config: %v", err)
	}
	if len(tags) != 0 {
		return tags
	}
	return nil
}
