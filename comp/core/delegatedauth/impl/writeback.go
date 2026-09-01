// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"maps"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// resolveTargetSite returns TargetSite if set, else AdditionalEndpointDomain, else empty (use primary site).
func resolveTargetSite(params delegatedauth.InstanceParams) string {
	if params.TargetSite != "" {
		return params.TargetSite
	}
	return params.AdditionalEndpointDomain
}

// fallbackTargetInstance builds a minimal authInstance for the no-cloud-provider case in AddInstance.
func fallbackTargetInstance(params delegatedauth.InstanceParams) *authInstance {
	return &authInstance{
		apiKeyConfigKey:                  params.APIKeyConfigKey,
		writebackPath:                    append([]string(nil), params.WritebackPath...),
		targetSite:                       resolveTargetSite(params),
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointKeyIndex:       params.AdditionalEndpointKeyIndex,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
	}
}

// updateConfigWithAPIKey updates the config with a newly-fetched, real (non-fallback) API key.
func (d *delegatedAuthComponent) updateConfigWithAPIKey(instance *authInstance, apiKey string) {
	d.writeAPIKeyToTarget(instance, apiKey, false)
}

// writeAPIKeyToTarget writes apiKey to the configured target (list-shape, map-shape, or flat key).
// isFallback only affects the log message.
func (d *delegatedAuthComponent) writeAPIKeyToTarget(instance *authInstance, apiKey string, isFallback bool) {
	switch {
	case len(instance.writebackPath) > 0:
		if err := pkgconfigmodel.AssignAtPath(d.config, instance.writebackPath, apiKey, pkgconfigmodel.SourceSecret); err != nil {
			log.Errorf("Could not write delegated auth key to config path %q: %v", instance.writebackPath, err)
			return
		}
		if isFallback {
			log.Infof("Using fallback API key at %q, ending with: %s", instance.writebackPath, scrubber.HideKeyExceptLastChars(apiKey))
		} else {
			log.Infof("Updated delegated API key at %q, ending with: %s", instance.writebackPath, scrubber.HideKeyExceptLastChars(apiKey))
		}
	case instance.additionalEndpointsListConfigKey != "":
		d.mergeIntoAdditionalEndpointsList(instance, apiKey, isFallback)
	case instance.additionalEndpointDomain != "":
		d.mergeIntoAdditionalEndpoints(instance, apiKey, isFallback)
	default:
		// Update the config value using the Writer interface
		// This will trigger OnUpdate callbacks for any components listening to this config
		d.config.Set(instance.apiKeyConfigKey, apiKey, pkgconfigmodel.SourceAgentRuntime)
		if isFallback {
			log.Infof("Using fallback API key for '%s' (delegated auth unavailable), ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastChars(apiKey))
		} else {
			log.Infof("Updated config key '%s' with new delegated API key ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastChars(apiKey))
		}
	}
}

// mergeIntoAdditionalEndpoints writes apiKey into the map-shape config at
// additionalEndpointsConfigKey under additionalEndpointDomain, replacing the previous value.
// Serialized via additionalEndpointsMu. Writes at SourceSecret (not SourceAgentRuntime) to avoid
// permanently shadowing secret rotations; the retry loop mitigates concurrent writes.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpoints(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsConfigKey
	domain := instance.additionalEndpointDomain

	written := false
	for attempt := 1; attempt <= maxAdditionalEndpointsWriteAttempts; attempt++ {
		endpoints := d.config.GetStringMapStringSlice(configKey)
		merged := make(map[string][]string, len(endpoints))
		for k, v := range endpoints {
			merged[k] = append([]string{}, v...)
		}

		keys := merged[domain]

		// Prefer the recorded index; fall back to a value-only scan if the list was reordered or
		// the index wasn't provided. Matching by value alone is ambiguous when another entry
		// under the same domain (e.g. a static key equal to this instance's fallback value)
		// happens to share the same string.
		matchIndex := -1
		if instance.additionalEndpointKeyIndex >= 0 && instance.additionalEndpointKeyIndex < len(keys) {
			if v := keys[instance.additionalEndpointKeyIndex]; v == instance.lastWrittenValue || v == instance.originalDirective {
				matchIndex = instance.additionalEndpointKeyIndex
			}
		}
		if matchIndex == -1 {
			for i, key := range keys {
				// Also match originalDirective in case a racing write reverted the entry.
				if key == instance.lastWrittenValue || key == instance.originalDirective {
					matchIndex = i
					break
				}
			}
		}

		replaced := matchIndex != -1
		if replaced {
			keys[matchIndex] = apiKey
		}
		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Expected value missing — concurrent writer may be mid-update. Retry.
				continue
			}
			// Unlike the list-shape path, appending here would orphan whatever key this
			// instance was tracking (it may be a live, unrelated key) rather than just
			// dropping this instance's own update.
			log.Warnf("Could not find previous delegated auth value for additional endpoint '%s' at '%s'; leaving domain's keys unchanged", domain, configKey)
			return
		}
		merged[domain] = keys

		// Re-check the whole value before writing to avoid discarding concurrent changes to other domains.
		if beforeWrite := d.config.GetStringMapStringSlice(configKey); !reflect.DeepEqual(beforeWrite, endpoints) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint '%s'; writing anyway after %d attempts", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck.
		if current := d.config.GetStringMapStringSlice(configKey); reflect.DeepEqual(current, merged) {
			written = true
			break
		}
		if lastAttempt {
			log.Warnf("Possible concurrent update to '%s' while writing delegated auth key for additional endpoint '%s'; giving up after %d attempts, a later refresh will retry", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}
	}

	// Only advance lastWrittenValue once the write is confirmed.
	if written {
		instance.lastWrittenValue = apiKey
	}
	if isFallback {
		log.Infof("Using fallback API key for additional endpoint '%s' at '%s' (delegated auth unavailable), ending with: %s", domain, configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint '%s' with new delegated API key ending with: %s", domain, scrubber.HideKeyExceptLastChars(apiKey))
	}
}

// mergeIntoAdditionalEndpointsList writes apiKey into the list-shape config at
// additionalEndpointsListConfigKey, replacing the entry matching lastWrittenValue.
// Locking, write source, and retry behavior mirror mergeIntoAdditionalEndpoints.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpointsList(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsListConfigKey

	for attempt := 1; attempt <= maxAdditionalEndpointsWriteAttempts; attempt++ {
		entries, ok := common.NormalizeListShapeEntries(d.config.Get(configKey))
		if !ok {
			log.Warnf("Could not read list-shape additional endpoints at '%s' (unexpected type); skipping delegated auth update", configKey)
			return
		}

		merged := make([]any, len(entries))
		copy(merged, entries)

		// Prefer the recorded index; fall back to a value-only scan if the list was reordered.
		// Non-map entries (preserved as-is by NormalizeListShapeEntries) can never match and are
		// skipped rather than causing a type-assertion panic.
		matchIndex := -1
		apiKeyField := ""
		if instance.listEntryIndex >= 0 && instance.listEntryIndex < len(entries) {
			if entryMap, ok := entries[instance.listEntryIndex].(map[string]any); ok {
				if field, valStr, ok := common.CaseInsensitiveStringFieldWithKey(entryMap, "api_key"); ok && (valStr == instance.lastWrittenValue || valStr == instance.originalDirective) {
					matchIndex = instance.listEntryIndex
					apiKeyField = field
				}
			}
		}
		if matchIndex == -1 {
			for i, entry := range entries {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				// Also match originalDirective in case a racing write reverted the entry.
				if field, valStr, ok := common.CaseInsensitiveStringFieldWithKey(entryMap, "api_key"); ok && (valStr == instance.lastWrittenValue || valStr == instance.originalDirective) {
					matchIndex = i
					apiKeyField = field
					break
				}
			}
		}

		replaced := matchIndex != -1
		if replaced {
			matchedEntry := entries[matchIndex].(map[string]any)
			newEntry := make(map[string]any, len(matchedEntry))
			maps.Copy(newEntry, matchedEntry)
			newEntry[apiKeyField] = apiKey
			merged[matchIndex] = newEntry
		}

		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Expected value missing — concurrent writer may be mid-update. Retry.
				continue
			}
			log.Warnf("Could not find previous delegated auth value in list-shape additional endpoints at '%s'; leaving list unchanged", configKey)
			return
		}

		// Re-check the list before writing — see mergeIntoAdditionalEndpoints.
		entriesNormalized, _ := common.NormalizeListShapeEntries(entries)
		if beforeWrite, ok := common.NormalizeListShapeEntries(d.config.Get(configKey)); ok && !reflect.DeepEqual(beforeWrite, entriesNormalized) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint entry; writing anyway after %d attempts", configKey, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck; normalize both sides since merged's element representation
		// isn't necessarily identical to what a fresh read of the same data produces.
		mergedNormalized, _ := common.NormalizeListShapeEntries(merged)
		if current, ok := common.NormalizeListShapeEntries(d.config.Get(configKey)); ok && reflect.DeepEqual(current, mergedNormalized) {
			instance.lastWrittenValue = apiKey
			break
		}
		if lastAttempt {
			log.Warnf("Possible concurrent update to '%s' while writing delegated auth key for additional endpoint entry; giving up after %d attempts, a later refresh will retry", configKey, maxAdditionalEndpointsWriteAttempts)
		}
	}

	if isFallback {
		log.Infof("Using fallback API key for additional endpoint entry at '%s' (delegated auth unavailable), ending with: %s", configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint entry at '%s' with new delegated API key ending with: %s", configKey, scrubber.HideKeyExceptLastChars(apiKey))
	}
}
