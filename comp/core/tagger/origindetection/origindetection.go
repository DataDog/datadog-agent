// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package origindetection contains the types and functions used for Origin Detection.
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

	// ExternalDataInitPrefix is the prefix for the Init flag in the External Data.
	ExternalDataInitPrefix = "it-"
	// ExternalDataContainerNamePrefix is the prefix for the Container Name in the External Data.
	ExternalDataContainerNamePrefix = "cn-"
	// ExternalDataPodUIDPrefix is the prefix for the Pod UID in the External Data.
	ExternalDataPodUIDPrefix = "pu-"
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

// GenerateContainerIDFromExternalData generates a container ID from the external data.
type GenerateContainerIDFromExternalData func(externalData ExternalData) (string, error)

// ParseExternalData parses the external data string into an ExternalData struct.
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
