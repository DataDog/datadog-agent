// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"encoding/json"
	"fmt"

	"github.com/samber/lo"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

	fields := lo.SliceToMap(images, func(image *wmdef.ContainerImageMetadata) (string, *wmdef.SBOM) {
		return image.ID, image.SBOM
	})

	// Using indent or splitting the file is necessary. Otherwise the scrubber will understand
	// the file as a single very large token and it will exceed the max buffer size.
	content, err := json.MarshalIndent(fields, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal results to JSON: %v", err)
	}

	_ = fb.AddFile("sbom.json", content)

	return nil
}
