// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrNoConfigVersion is returned when a config is missing its version in its custom meta
	ErrNoConfigVersion = errors.New("config has no version in its meta")
)

type errUnknownProduct struct {
	product string
}

func (e *errUnknownProduct) Error() string {
	return fmt.Sprintf("unknown product %s", e.product)
}

type fileMetaCustom struct {
	Version *uint64  `json:"v"`
	Clients []string `json:"c"`
	Expire  int64    `json:"e"`
}

func parseFileMetaCustom(rawCustom []byte) (fileMetaCustom, error) {
	var custom fileMetaCustom
	err := json.Unmarshal(rawCustom, &custom)
	if err != nil {
		return fileMetaCustom{}, err
	}
	if custom.Version == nil {
		return fileMetaCustom{}, ErrNoConfigVersion
	}
	return custom, nil
}
