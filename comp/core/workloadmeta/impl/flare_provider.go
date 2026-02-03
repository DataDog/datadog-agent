// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/samber/lo"
)

/*
This file contains the implementation of the flare providers for the workloadmeta component.
The flare providers are used to add additional data to the flare archive. They should be provided
with workloadmeta to dump its state.
*/

// sbomFlareProvider will add the SBOMs of all the images in the flare archive.
// Note that the generated file uncompressed can be very large
func (w *workloadmeta) sbomFlareProvider(fb flaretypes.FlareBuilder) error {
	images := w.ListImages()
	names := make(map[string]int)

	for _, image := range images {
		sbom, err := sbomutil.UncompressSBOM(image.SBOM)
		if err != nil {
			log.Errorf("Failed to uncompress SBOM for image %s: %v", image.ID, err)
			continue
		}

		content, err := json.MarshalIndent(sbom, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to marshal results to JSON: %v", err)
		}

		name := idToFileSafe(image.ID)

		// just in case multiple images have the same ID, let's make the name unique
		counter := names[name]
		if counter != 0 {
			name = fmt.Sprintf("%s_%d", name, counter)
		}
		names[name]++

		_ = fb.AddFileWithoutScrubbing(filepath.Join("sbom", name+".json"), content)
	}

	if w.config.GetBool("runtime_security_config.sbom.enabled") {
		containers := w.ListContainers()

		fields := lo.SliceToMap(containers, func(container *wmdef.Container) (string, *wmdef.SBOM) {
			return container.ID, container.SBOM
		})

		content, err := json.MarshalIndent(fields, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to marshal results to JSON: %v", err)
		}

		_ = fb.AddFile("containers-sbom.json", content)
	}

	return nil
}

var invalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func idToFileSafe(id string) string {
	// replace invalid characters with underscores
	return invalidChars.ReplaceAllString(id, "_")
}
