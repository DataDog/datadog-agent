// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"fmt"
	"os"
	"strings"
)

func GetBaseTags() []string {
	if len(os.Getenv("K_SERVICE")) > 0 && len(os.Getenv("K_REVISION")) > 0 {
		return []string{
			fmt.Sprintf("revision:%s", strings.ToLower(os.Getenv("K_REVISION"))),
			fmt.Sprintf("service:%s", strings.ToLower(os.Getenv("K_SERVICE"))),
		}
	}
	return []string{}
}
