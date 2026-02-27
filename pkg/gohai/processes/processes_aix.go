// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package processes

import "errors"

func getProcessGroups(_ int) ([]ProcessGroup, error) {
	return nil, errors.New("process groups not supported on AIX")
}
