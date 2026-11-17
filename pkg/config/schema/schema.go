// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package schema embeds the Agent configuration schemas and provides a function to retrieve them. Validating a
// configuration against them is done by the schema component (comp/core/schema).
package schema

import (
	"embed"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

//go:embed all:compressed
var schemas embed.FS

func getRawSchema(name string) ([]byte, error) {
	data, err := schemas.ReadFile("compressed/" + name + ".yaml.zstd")
	if err != nil {
		return nil, fmt.Errorf("no embedded schema for %s", name)
	}

	uncompressSchema, err := zstd.Decompress(nil, data)
	if err != nil {
		return nil, fmt.Errorf("could not Decompress schema '%s': %s", name, err)
	}

	return uncompressSchema, nil
}

// GetRawSchema returns the raw bytes of an embedded schema.
func GetRawSchema(ID model.SchemaID) ([]byte, error) {
	if ID == model.CoreSchema {
		return getRawSchema("core_schema")
	} else if ID == model.SystemProbeSchema {
		return getRawSchema("system-probe_schema")
	}
	return nil, fmt.Errorf("unknown schema ID '%s'", ID)
}
