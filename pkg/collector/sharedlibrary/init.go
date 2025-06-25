// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"  -I "${SRCDIR}/../../../rtloader/common"
*/
import "C"

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var (
	rtloader *C.rtloader_t
)

// add the shared library loader to the scheduler
// the loader needs to be registered in this function otherwise it won't be listed when we load a check using the CLI
func init() {
	factory := func(sender.SenderManager, option.Option[integrations.Component], tagger.Component) (check.Loader, int, error) {
		loader, err := NewSharedLibraryCheckLoader()
		priority := 40
		return loader, priority, err
	}

	loaders.RegisterLoader(factory)
}

// useless for now, will be used later
func initLoaderConfig() {
	// get rtloader shared library object pointer
	//rtloader = C.init_shared_library() // can't implement this now, see api.cpp to understand why
}

// initSharedLibrary initializes the shared library loader if enabled
func InitSharedLibrary() {
	if pkgconfigsetup.Datadog().GetBool("shared_library_checks") {
		initLoaderConfig()
	} else {
		log.Warn("Shared Library checks are disabled.")
	}
}
