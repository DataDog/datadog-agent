// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/helpers"
)

const (
	KubeAllowedEncodeStringAlphaNums = helpers.KubeAllowedEncodeStringAlphaNums
	Digits                           = helpers.Digits
)

var (
	ParseDeploymentForReplicaSet = helpers.ParseDeploymentForReplicaSet
	ParseCronJobForJob           = helpers.ParseCronJobForJob
)
