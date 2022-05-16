package service

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Masterminds/semver"
	"github.com/theupdateframework/go-tuf/data"
)

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

	for path, meta := range directorTargets {
		tracerPredicates, err := parsePredicates(meta.Custom)
		if err != nil {
			return nil, err
		}

		var matched bool
		nullPredicates := tracerPredicates == nil || tracerPredicates.TracerPredicates == nil
		if !nullPredicates {
			if tracerPredicates.Version != 0 {
				log.Infof("Unsupported predicate version %d for products %s", tracerPredicates.Version)
				continue
			}
			matched, err = executePredicate(client, tracerPredicates.TracerPredicates)
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

func executePredicate(client *pbgo.Client, predicates []*pbgo.TracerPredicate) (bool, error) {
	for _, predicate := range predicates {
		if client.IsTracer {
			tracer := client.ClientTracer
			if predicate.RuntimeID != "" && tracer.RuntimeId != predicate.RuntimeID {
				return false, nil
			}

			if predicate.Service != "" && tracer.Service != predicate.Service {
				return false, nil
			}

			if predicate.Environment != "" && tracer.Env != predicate.Environment {
				return false, nil
			}

			if predicate.Language != "" && tracer.Language != predicate.Language {
				return false, nil
			}

			if predicate.AppVersion != "" && tracer.AppVersion != predicate.AppVersion {
				return false, nil
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
				if len(errs) != 0 {
					return false, fmt.Errorf("errors: %s", errs)
				}
				if !matched || len(errs) > 0 {
					return false, nil
				}
			}
		}
	}

	return true, nil
}
