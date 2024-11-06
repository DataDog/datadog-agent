// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common provides common utilities that are useful when interacting with the tagger.
package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// BuildTaggerEntityID builds tagger entity id based on workloadmeta entity id
func BuildTaggerEntityID(entityID workloadmeta.EntityID) types.EntityID {
	switch entityID.Kind {
	case workloadmeta.KindContainer:
		return types.NewEntityID(types.ContainerID, entityID.ID)
	case workloadmeta.KindKubernetesPod:
		return types.NewEntityID(types.KubernetesPodUID, entityID.ID)
	case workloadmeta.KindECSTask:
		return types.NewEntityID(types.ECSTask, entityID.ID)
	case workloadmeta.KindContainerImageMetadata:
		return types.NewEntityID(types.ContainerImageMetadata, entityID.ID)
	case workloadmeta.KindProcess:
		return types.NewEntityID(types.Process, entityID.ID)
	case workloadmeta.KindKubernetesDeployment:
		return types.NewEntityID(types.KubernetesDeployment, entityID.ID)
	case workloadmeta.KindKubernetesMetadata:
		return types.NewEntityID(types.KubernetesMetadata, entityID.ID)
	default:
		log.Errorf("can't recognize entity %q with kind %q; trying %s://%s as tagger entity",
			entityID.ID, entityID.Kind, entityID.ID, entityID.Kind)
		return types.NewEntityID(types.EntityIDPrefix(entityID.Kind), entityID.ID)
	}
}

var globalEntityID = types.NewEntityID("internal", "global-entity-id")

// GetGlobalEntityID returns the entity ID that holds global tags
func GetGlobalEntityID() types.EntityID {
	return globalEntityID
}

// ExtractPrefixAndID extracts prefix and id from tagger entity id and returns an error if the received entityID is not valid
func ExtractPrefixAndID(entityID string) (prefix types.EntityIDPrefix, id string, err error) {
	extractedPrefix, extractedID, found := strings.Cut(entityID, "://")
	if !found {
		return "", "", fmt.Errorf("unsupported tagger entity id format %q, correct format is `{prefix}://{id}`", entityID)
	}

	return types.EntityIDPrefix(extractedPrefix), extractedID, nil
}
