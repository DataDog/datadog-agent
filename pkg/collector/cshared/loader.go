// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

// #cgo LDFLAGS: -ldl
// #include <dlfcn.h>
// #include <stdlib.h>
//
// void *get_check_factory(char* checklib) {
//     void *lib = dlopen(checklib, RTLD_LAZY);
//     if (lib == 0) {
//         return 0;
//     }
//     void *factory_func = dlsym(lib, "CheckFactory");
//     if (factory_func == 0) {
//         return 0;
//     }
//     return ((void*(*)()) factory_func)();
// }
import "C"

import (
	"errors"
	"fmt"
	"sync"
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

var factoriesLock sync.Mutex
var factories = make(map[string]option.Option[func() check.Check])

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
	factoriesLock.Lock()
	defer factoriesLock.Unlock()

	if _, ok := factories[config.Name]; !ok {
		libname := fmt.Sprintf("libcheck%s.so", config.Name)
		log.Infof("getting check factory from %s", libname)

		libnameC := C.CString(libname)
		var factoryFuncPtr unsafe.Pointer = C.get_check_factory(libnameC)
		C.free(unsafe.Pointer(libnameC))

		if factoryFuncPtr == nil {
			errmsg := C.dlerror()
			return nil, fmt.Errorf("could not load check %s from %s: %s", config.Name, libname, C.GoString(errmsg))
		}
		log.Infof("successfully loaded check factory from %s", libname)

		factoryFunc := *(*func() option.Option[func() check.Check])(factoryFuncPtr)
		factories[config.Name] = factoryFunc()
	}

	factory := factories[config.Name]
	checkFunc, ok := factory.Get()
	if !ok {
		return nil, fmt.Errorf("Check %s not found in Catalog", config.Name)
	}

	c := checkFunc()
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
