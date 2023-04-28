// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"fmt"

	infraaws "github.com/DataDog/test-infra-definitions/aws"
	commonconfig "github.com/DataDog/test-infra-definitions/common/config"
)

func GetDefaultKeyPairParamName() string {
	return fmt.Sprintf("%v:%v", commonconfig.DDInfraConfigNamespace, infraaws.DDInfraDefaultKeyPairParamName)
}
