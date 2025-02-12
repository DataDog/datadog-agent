// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"maps"
	"slices"
	"sync"
	"time"
)

var rcSingleton *UpdatableProvider
var rcOnce sync.Once
var rcError error

// NewRCProvider returns a profile provider that subscribes to remote
// configuration and receives profile updates from the backend. Multiple calls
// will return the same singleton object.
func NewRCProvider(client rcclient.Component) (Provider, error) {
	rcOnce.Do(func() {
		rcSingleton, rcError = buildAndSubscribeRCProvider(client)
	})
	return rcSingleton, rcError
}

// buildAndSubscribeRCProvider builds a new UpdatableProvider and subscribes to
// RC to receive profile updates.
func buildAndSubscribeRCProvider(rcClient rcclient.Component) (*UpdatableProvider, error) {
	// Load OOTB profiles from YAML
	defaultProfiles := getYamlDefaultProfiles()
	if defaultProfiles == nil {
		return nil, fmt.Errorf("could not find OOTB profiles")
	}
	userProfiles := make(ProfileConfigMap)

	provider := &UpdatableProvider{}
	provider.Update(userProfiles, defaultProfiles, time.Now())

	// Subscribe to the RC client
	log.Debug("Subscribing to remote config profiles")
	rcClient.Subscribe(state.ProductNDMDeviceProfilesCustom, makeOnUpdate(provider))

	return provider, nil
}

// unpackRawConfigs converts a map of raw remote config data to a map of parsed
// profiles.
func unpackRawConfigs(update map[string]state.RawConfig) (ProfileConfigMap, map[string]error) {
	errors := make(map[string]error)
	profiles := make(ProfileConfigMap)
	// iterate over keys in sorted order for determinism
	keys := slices.Sorted(maps.Keys(update))
	for _, k := range keys {
		v := update[k]
		var def profiledefinition.DeviceProfileRcConfig
		err := json.Unmarshal(v.Config, &def)
		if err != nil {
			err = fmt.Errorf("could not unmarshal device profile config %q: %w", k, err)
			_ = log.Warn(err)
			errors[k] = err
			continue
		}
		if _, ok := profiles[def.Profile.Name]; ok {
			_ = log.Warnf("Received multiple profiles for name: %q", def.Profile.Name)
			errors[k] = fmt.Errorf("multiple profiles for name: %q", def.Profile.Name)
			continue
		}
		profiles[def.Profile.Name] = ProfileConfig{
			DefinitionFile: "",
			Definition:     def.Profile,
			IsUserProfile:  true,
		}
	}
	return profiles, errors
}

// makeOnUpdate generates an onUpdate function suitable for rcclient.Component.
// Subscribe that will update the given UpdatableProvider whenever the RC client
// receives new profiles.
func makeOnUpdate(up *UpdatableProvider) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	onUpdate := func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		log.Debugf("Received %d profiles via remote configuration", len(update))
		userProfiles, errors := unpackRawConfigs(update)
		// update is a dict of ALL current custom profiles, so we replace the existing set entirely.
		up.Update(userProfiles, up.defaultProfiles, time.Now())
		// Report successes/errors
		for k := range update {
			if errors[k] != nil {
				applyStateCallback(k, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: errors[k].Error(),
				})
			} else {
				applyStateCallback(k, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			}
		}
	}
	return onUpdate
}
