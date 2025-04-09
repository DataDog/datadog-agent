// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinit

import (
	"fmt"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

// #include <datadog_agent_rtloader.h>
//
import "C"

// getRtLoader returns a new rtloader instance
func getRtLoader() (*C.rtloader_t, error) {
	rtloader := (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return nil, fmt.Errorf("make failed")
	}
	return rtloader, nil
}

// runInitWithPath initializes the rtloader with the given Python path
func runInit(pythonPath string) error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader, err := getRtLoader()
	if err != nil {
		return err
	}

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString(pythonPath))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}
