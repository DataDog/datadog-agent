// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"encoding/json"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/Masterminds/semver"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

// ConfigFileMetaCustom is the custom metadata of a config
type ConfigFileMetaCustom struct {
	Predicates *pbgo.TracerPredicates `json:"tracer-predicates,omitempty"`
	Expires    int64                  `json:"expires"`
}

// Given the hostname and state will parse predicates and execute them
// It will return a list
func executeTracerPredicates(
	client *pbgo.Client,
	directorTargets data.TargetFiles,
) ([]string, error) {
	configs := make([]string, 0)

	productsMap := make(map[string]struct{})
	for _, product := range client.Products {
		productsMap[product] = struct{}{}
	}

	for path, meta := range directorTargets {
		pathMeta, err := rdata.ParseConfigPath(path)
		if err != nil {
			return nil, err
		}
		if _, productRequested := productsMap[pathMeta.Product]; !productRequested {
			continue
		}
		configMetadata, err := parseFileMetaCustom(meta.Custom)
		if err != nil {
			return nil, err
		}
		if configExpired(configMetadata.Expires) {
			continue
		}

		tracerPredicates := configMetadata.Predicates
		matched := false
		nullPredicates := tracerPredicates == nil || tracerPredicates.TracerPredicatesV1 == nil
		if !nullPredicates {
			matched, err = executePredicate(client, tracerPredicates.TracerPredicatesV1)
			if err != nil {
				return nil, err
			}
		}

		if matched || nullPredicates {
			configs = append(configs, path)
		}

	}

	return configs, nil
}

func parseFileMetaCustom(customJSON *json.RawMessage) (ConfigFileMetaCustom, error) {
	var metadata ConfigFileMetaCustom

	if customJSON == nil {
		return metadata, nil
	}

	err := json.Unmarshal(*customJSON, &metadata)
	if err != nil {
		return metadata, err
	}

	return metadata, nil
}

func executePredicate(client *pbgo.Client, predicates []*pbgo.TracerPredicateV1) (bool, error) {
	// No tracer predicates match everything
	if len(predicates) == 0 {
		return true, nil
	}
	for _, predicate := range predicates {
		if predicate.ClientID != "" && client.Id != predicate.ClientID {
			continue
		}
		if client.IsTracer {
			tracer := client.ClientTracer
			if predicate.RuntimeID != "" && tracer.RuntimeId != predicate.RuntimeID {
				continue
			}

			if predicate.Service != "" && tracer.Service != predicate.Service {
				continue
			}

			if predicate.Environment != "" && tracer.Env != predicate.Environment {
				continue
			}

			if predicate.Language != "" && tracer.Language != predicate.Language {
				continue
			}

			if predicate.AppVersion != "" && tracer.AppVersion != predicate.AppVersion {
				continue
			}

			if predicate.TracerVersion != "" {
				version, err := semver.NewVersion(tracer.TracerVersion)
				if err != nil {
					return false, err
				}
				versionConstraint, err := semver.NewConstraint(predicate.TracerVersion)
				if err != nil {
					return false, err
				}

				matched, errs := versionConstraint.Validate(version)
				// We don't return on error here, it's simply the version not matching
				if !matched || len(errs) > 0 {
					continue
				}
			}
		}
		return true, nil
	}

	return false, nil
}

func configExpired(expiration int64) bool {
	// A value of 0 means "no expiration"
	if expiration == 0 {
		return false
	}

	expirationTime := time.Unix(expiration, 0)

	return time.Now().After(expirationTime)
}
