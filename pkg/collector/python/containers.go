// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
)

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static
*/
import "C"

// IsContainerExcluded returns whether a container should be excluded,
// based on it's name, image name and namespace. Exclusion patterns are configured
// via the global options (ac_include/ac_exclude/exclude_pause_container)
//
//export IsContainerExcluded
func IsContainerExcluded(name, image, namespace *C.char) C.int {
	checkContext, err := getCheckContext()
	if err != nil {
		return 0
	}

	goName := C.GoString(name)
	goImg := C.GoString(image)
	goNs := ""
	if namespace != nil {
		goNs = C.GoString(namespace)
	}

	filterablePod := &workloadfilter.Pod{
		FilterPod: &typedef.FilterPod{
			Id:          "",
			Name:        "",
			Namespace:   goNs,
			Annotations: map[string]string{},
		},
	}
	filterableContainer := &workloadfilter.Container{
		FilterContainer: &typedef.FilterContainer{
			Id:    "",
			Name:  goName,
			Image: goImg,
		},
		Owner: filterablePod,
	}

	if checkContext.filter.IsExcluded(filterableContainer) {
		return 1
	}
	return 0
}
