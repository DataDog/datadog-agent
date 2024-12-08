// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package origindetection

import (
	"strconv"
	"strings"
)

// ProductOrigin is the origin of the product that sent the entity.
type ProductOrigin int

const (
	// ProductOriginDogStatsDLegacy is the ProductOrigin for DogStatsD in Legacy mode.
	// TODO: remove this when dogstatsd_origin_detection_unified is enabled by default
	ProductOriginDogStatsDLegacy ProductOrigin = iota
	// ProductOriginDogStatsD is the ProductOrigin for DogStatsD.
	ProductOriginDogStatsD ProductOrigin = iota
	// ProductOriginAPM is the ProductOrigin for APM.
	ProductOriginAPM ProductOrigin = iota

	// External Data Prefixes
	// These prefixes are used to build the External Data Environment Variable.

	ExternalDataInitPrefix          = "it-"
	ExternalDataContainerNamePrefix = "cn-"
	ExternalDataPodUIDPrefix        = "pu-"
)

// OriginInfo contains the Origin Detection information.
type OriginInfo struct {
	ContainerIDFromSocket string        // ContainerIDFromSocket is the origin resolved using Unix Domain Socket.
	PodUID                string        // PodUID is the origin resolved from the Kubernetes Pod UID.
	ContainerID           string        // ContainerID is the origin resolved from the container ID.
	ExternalData          ExternalData  // ExternalData is the external data list.
	Cardinality           string        // Cardinality is the cardinality of the resolved origin.
	ProductOrigin         ProductOrigin // ProductOrigin is the product that sent the origin information.
}

// ExternalData contains the parsed external data items.
type ExternalData struct {
	Init          bool
	ContainerName string
	PodUID        string
}

type GenerateContainerIDFromExternalData func(externalData ExternalData) (string, error)

func ParseExternalData(externalEnv string) (ExternalData, error) {
	if externalEnv == "" {
		return ExternalData{}, nil
	}
	var externalData ExternalData
	var parsingError error
	for _, item := range strings.Split(externalEnv, ",") {
		switch {
		case strings.HasPrefix(item, ExternalDataInitPrefix):
			externalData.Init, parsingError = strconv.ParseBool(item[len(ExternalDataInitPrefix):])
		case strings.HasPrefix(item, ExternalDataContainerNamePrefix):
			externalData.ContainerName = item[len(ExternalDataContainerNamePrefix):]
		case strings.HasPrefix(item, ExternalDataPodUIDPrefix):
			externalData.PodUID = item[len(ExternalDataPodUIDPrefix):]
		}
	}
	return externalData, parsingError
}
