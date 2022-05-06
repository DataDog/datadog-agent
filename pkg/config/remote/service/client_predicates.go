package service

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/Masterminds/semver"
	"github.com/theupdateframework/go-tuf/data"
)

type ClientPredicate struct {
	ClientID      string `json:"client_id,omitempty"`
	Service       string `json:"service,omitempty"`
	Environment   string `json:"environment,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	TracerVersion string `json:"tracer_version,omitempty"`
	Product       string `json:"product,omitempty"`
	Language      string `json:"language,omitempty"`
}

type Predicates struct {
	Version    int                `json:"version,omitempty"`
	Predicates []*ClientPredicate `json:"predicates,omitempty"`
}

type DirectorTargetsCustomMetadata struct {
	Predicates *Predicates `json:"predicates,omitempty"`
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

func parsePredicates(customJSON *json.RawMessage) (*Predicates, error) {
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

func executePredicate(client *pbgo.Client, predicates []*ClientPredicate) (bool, error) {
	for _, predicate := range predicates {
		if client.IsTracer {
			tracer := client.ClientTracer
			if predicate.ClientID != "" {
				if tracer.RuntimeId != predicate.ClientID {
					return false, nil
				}
			}

			if predicate.Service != "" {
				if tracer.Service != predicate.Service {
					return false, nil
				}
			}

			if predicate.Environment != "" {
				if tracer.Env != predicate.Environment {
					return false, nil
				}
			}

			if predicate.Language == "" {
				if tracer.Language != predicate.Language {
					return false, nil
				}
			}

			if predicate.AppVersion != "" {
				if predicate.AppVersion != tracer.AppVersion {
					return false, nil
				}
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
				if !matched || errs != nil {
					return false, fmt.Errorf("errors: %s", errs)
				}
			}

		}

		if predicate.Product != "" {
			var anyMatch bool
			for _, p := range client.Products {
				if p == predicate.Product {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				return false, nil
			}
		}

	}

	return true, nil
}
