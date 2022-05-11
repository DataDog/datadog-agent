package service

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/Masterminds/semver"
	"github.com/theupdateframework/go-tuf/data"
)

type clientPredicate struct {
	RuntimeID     *string `json:"runtime-id,omitempty"`
	Service       *string `json:"service,omitempty"`
	Environment   *string `json:"environment,omitempty"`
	AppVersion    *string `json:"app-version,omitempty"`
	TracerVersion *string `json:"tracer-version,omitempty"`
	Language      *string `json:"language,omitempty"`
}

type clientPredicates struct {
	Version    int                `json:"version,omitempty"`
	Predicates []*clientPredicate `json:"predicates,omitempty"`
}

type DirectorTargetsCustomMetadata struct {
	Predicates *clientPredicates `json:"tracer-predicates,omitempty"`
}

// Given the hostname and state will parse predicates and execute them
// It will return a list ConfigPointers
func executeClientPredicates(
	client *pbgo.Client,
	directorTargets data.TargetFiles,
) ([]*pbgo.ConfigPointer, error) {
	configPointers := make([]*pbgo.ConfigPointer, 0)

	for path, meta := range directorTargets {
		predicates, err := parsePredicates(meta.Custom)
		if err != nil {
			return nil, err
		}

		var matched bool
		nullPredicates := predicates == nil || predicates.Predicates == nil
		if !nullPredicates {
			matched, err = executePredicate(client, predicates.Predicates)
			if err != nil {
				return nil, err
			}
		}

		if matched || nullPredicates {
			configPointers = append(configPointers, &pbgo.ConfigPointer{Path: path})
		}

	}

	return configPointers, nil
}

func parsePredicates(customJSON *json.RawMessage) (*clientPredicates, error) {
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

func executePredicate(client *pbgo.Client, predicates []*clientPredicate) (bool, error) {
	for _, predicate := range predicates {
		if client.IsTracer {
			tracer := client.ClientTracer
			if predicate.RuntimeID != nil {
				if tracer.RuntimeId != *predicate.RuntimeID {
					return false, nil
				}
			}

			if predicate.Service != nil {
				if tracer.Service != *predicate.Service {
					return false, nil
				}
			}

			if predicate.Environment != nil {
				if tracer.Env != *predicate.Environment {
					return false, nil
				}
			}

			if predicate.Language != nil {
				if tracer.Language != *predicate.Language {
					return false, nil
				}
			}

			if predicate.AppVersion != nil {
				if tracer.AppVersion != *predicate.AppVersion {
					return false, nil
				}
			}

			if predicate.TracerVersion != nil {
				version, err := semver.NewVersion(tracer.TracerVersion)
				if err != nil {
					return false, err
				}
				versionConstraint, err := semver.NewConstraint(*predicate.TracerVersion)
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
