// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package processes

import "errors"

func getProcessGroups(_ int) ([]ProcessGroup, error) {
	return nil, errors.New("Not implemented on Windows")
}
