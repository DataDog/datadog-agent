// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"encoding/json"

	"github.com/Masterminds/semver"
	"github.com/theupdateframework/go-tuf/data"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

// DirectorTargetsCustomMetadata TODO (<remote-config>): RCM-228
type DirectorTargetsCustomMetadata struct {
	Predicates *pbgo.TracerPredicates `json:"tracer-predicates,omitempty"`
}

// Given the hostname and state will parse predicates and execute them
// It will return a list
func executeClientPredicates(
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
		tracerPredicates, err := parsePredicates(meta.Custom)
		if err != nil {
			return nil, err
		}

		var matched bool
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

func parsePredicates(customJSON *json.RawMessage) (*pbgo.TracerPredicates, error) {
	if customJSON == nil {
		return nil, nil
	}
	metadata := new(DirectorTargetsCustomMetadata)
	err := json.Unmarshal(*customJSON, metadata)
	if err != nil {
		return nil, err
	}
	return metadata.Predicates, nil
}

func executePredicate(client *pbgo.Client, predicates []*pbgo.TracerPredicateV1) (bool, error) {
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
