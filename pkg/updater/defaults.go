// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

var (
	//go:embed defaults/bootstrap.json
	rawDefaultBootstrapVersions []byte
	//go:embed defaults/catalog.json
	rawDefaultCatalog []byte

	defaultBootstrapVersions bootstrapVersions
	defaultCatalog           catalog
)

func init() {
	err := json.Unmarshal(rawDefaultBootstrapVersions, &defaultBootstrapVersions)
	if err != nil {
		panic(fmt.Sprintf("could not unmarshal default bootstrap versions: %s", err))
	}
	err = json.Unmarshal(rawDefaultCatalog, &defaultCatalog)
	if err != nil {
		panic(fmt.Sprintf("could not unmarshal default catalog: %s", err))
	}
}
