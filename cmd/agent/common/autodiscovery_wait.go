// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.uber.org/atomic"
	utilserror "k8s.io/apimachinery/pkg/util/errors"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
)

// schedulerFunc is a type alias to allow a function to be used as an AD scheduler
type schedulerFunc func([]integration.Config)

// Schedule implements scheduler.Scheduler#Schedule.
func (sf schedulerFunc) Schedule(configs []integration.Config) {
	sf(configs)
}

// Unschedule implements scheduler.Scheduler#Unschedule.
func (sf schedulerFunc) Unschedule(_ []integration.Config) {
	// (do nothing)
}

// Stop implements scheduler.Scheduler#Stop.
func (sf schedulerFunc) Stop() {
}

// WaitForConfigsFromAD waits until a count of discoveryMinInstances configs
// with names in checkNames are scheduled by AD, and returns the matches.
//
// If the context is cancelled, then any accumulated, matching changes are
// returned, even if that is fewer than discoveryMinInstances.
func WaitForConfigsFromAD(ctx context.Context,
	checkNames []string,
	discoveryMinInstances int,
	instanceFilter string,
	ac autodiscovery.Component,
) (configs []integration.Config, lastError error) {
	return waitForConfigsFromAD(ctx, false, checkNames, discoveryMinInstances, instanceFilter, ac)
}

// WaitForAllConfigsFromAD waits until its context expires, and then returns
// the full set of checks scheduled by AD.
func WaitForAllConfigsFromAD(ctx context.Context, ac autodiscovery.Component) (configs []integration.Config, lastError error) {
	return waitForConfigsFromAD(ctx, true, []string{}, 0, "", ac)
}

// waitForConfigsFromAD waits for configs from the AD scheduler and returns them.
//
// AD scheduling is asynchronous, so this is a time-based process.
//
// If wildcard is false, this waits until at least discoveryMinInstances
// configs with names in checkNames are scheduled by AD, and returns the
// matches.  If the context is cancelled before that occurs, then any
// accumulated configs are returned, even if that is fewer than
// discoveryMinInstances.
//
// If wildcard is true, this gathers all configs scheduled before the context
// is cancelled, and then returns.  It will not return before the context is
// cancelled.
func waitForConfigsFromAD(ctx context.Context,
	wildcard bool,
	checkNames []string,
	discoveryMinInstances int,
	instanceFilter string,
	ac autodiscovery.Component,
) (configs []integration.Config, returnErr error) {
	configChan := make(chan integration.Config)

	// signal to the scheduler when we are no longer waiting, so we do not continue
	// to push items to configChan
	waiting := atomic.NewBool(true)
	defer func() {
		waiting.Store(false)
		// ..and drain any message currently pending in the channel
		select {
		case <-configChan:
		default:
		}
	}()

	var match func(cfg integration.Config) bool
	if wildcard {
		// match all configs
		match = func(integration.Config) bool { return true }
	} else {
		// match configs with names in checkNames
		match = func(cfg integration.Config) bool {
			return slices.Contains(checkNames, cfg.Name)
		}
	}

	stopChan := make(chan struct{})
	// add the scheduler in a goroutine, since it will schedule any "catch-up" immediately,
	// placing items in configChan
	go ac.AddScheduler(adtypes.CheckCmdName, schedulerFunc(func(configs []integration.Config) {
		var errorList []error
		for _, cfg := range configs {
			if instanceFilter != "" {
				instances, filterErrors := filterInstances(cfg.Instances, instanceFilter)
				if len(filterErrors) > 0 {
					errorList = append(errorList, filterErrors...)
					continue
				}
				if len(instances) == 0 {
					continue
				}
				cfg.Instances = instances
			}

			if match(cfg) && waiting.Load() {
				configChan <- cfg
			}
		}
		if len(errorList) > 0 {
			returnErr = errors.New(utilserror.NewAggregate(errorList).Error())
			stopChan <- struct{}{}
		}
	}), true)

	for wildcard || len(configs) < discoveryMinInstances {
		select {
		case cfg := <-configChan:
			configs = append(configs, cfg)
		case <-stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
	return
}

func filterInstances(instances []integration.Data, instanceFilter string) ([]integration.Data, []error) {
	var newInstances []integration.Data
	var errors []error
	for _, instance := range instances {
		exist, err := jsonquery.YAMLCheckExist(instance, instanceFilter)
		if err != nil {
			errors = append(errors, fmt.Errorf("instance filter error: %v", err))
			continue
		}
		if exist {
			newInstances = append(newInstances, instance)
		}
	}
	return newInstances, errors
}
