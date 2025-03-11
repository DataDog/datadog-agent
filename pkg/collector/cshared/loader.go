// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

// #cgo LDFLAGS: -ldl
// #include <dlfcn.h>
// #include <stdlib.h>
//
// void get_check_from_lib(char *checklib, char *loadFuncName, void **ret) {
//     void *lib = dlopen(checklib, RTLD_LAZY);
//     if (lib == 0) {
//         return;
//     }
//     void *factory_func = dlsym(lib, loadFuncName);
//     if (factory_func == 0) {
//         return;
//     }
//     ((void(*)(void **)) factory_func)(ret);
// }
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type csharedLoader struct{}

// Initialize registers the cshared loader
func Initialize() {
	loaders.RegisterLoader(40, func(sender.SenderManager, option.Option[integrations.Component], tagger.Component) (check.Loader, error) {
		return newCsharedLoader()
	})
}

func (*csharedLoader) Name() string {
	return "cshared"
}

func (*csharedLoader) String() string {
	return "Go Cshared Loader"
}

func (*csharedLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data) (check.Check, error) {
	log.Infof("loading check %s", config.Name)

	libname := fmt.Sprintf("libcheck%s.so", config.Name)
	log.Infof("getting check factory from %s", libname)

	libnameC := C.CString(libname)
	loadCheckFuncName := C.CString(fmt.Sprintf("%sLoadCheck", config.Name))
	var checkPtr unsafe.Pointer
	C.get_check_from_lib(libnameC, loadCheckFuncName, &checkPtr)
	C.free(unsafe.Pointer(libnameC))
	C.free(unsafe.Pointer(loadCheckFuncName))

	if checkPtr == nil {
		errmsg := C.dlerror()
		return nil, fmt.Errorf("could not load check %s from %s: %s", config.Name, libname, C.GoString(errmsg))
	}
	log.Infof("successfully loaded check from %s", libname)
	log.Flush()

	c := *(*check.Check)(checkPtr)

	log.Infof("configuring check %s", c)
	if err := c.Configure(senderManager, config.FastDigest(), instance, config.InitConfig, config.Source); err != nil {
		if errors.Is(err, check.ErrSkipCheckInstance) {
			return c, err
		}
		log.Errorf("cshared.loader: could not configure check %s: %s", c, err)
		msg := fmt.Sprintf("Could not configure check %s: %s", c, err)
		return c, errors.New(msg)
	}

	return c, nil
}

func newCsharedLoader() (check.Loader, error) {
	return &csharedLoader{}, nil
}
