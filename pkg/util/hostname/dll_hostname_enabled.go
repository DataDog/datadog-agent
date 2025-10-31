// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && dll_hostname && !serverless

//go:generate bash -c "cd ${SRCDIR}/rustlib && cargo build --release"

package hostname

import (
	"context"
	"fmt"
	"unsafe"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#cgo LDFLAGS: -L${SRCDIR}/rustlib/target/release -ldd_dll_hostname
#include <stdlib.h>

// FFI functions provided by the shared library
extern char* dd_hostname(const char* provider, const char* hostname_file);
extern void dd_dll_free(char*);
*/
import "C"

// dllResolveHostname calls the Rust FFI resolver with the given provider key
// (e.g., "os", "fqdn"), validates the returned pointer and string, and
// returns the resolved hostname or an error.
func dllResolveHostname(providerName string, hostnameFile string) (string, error) {
	cProvider := C.CString(providerName)
	defer C.free(unsafe.Pointer(cProvider))

	var cHostnameFile *C.char
	if hostnameFile != "" {
		cHostnameFile = C.CString(hostnameFile)
		defer C.free(unsafe.Pointer(cHostnameFile))
	}

	ptr := C.dd_hostname(cProvider, cHostnameFile)
	if ptr == nil {
		return "", fmt.Errorf("dll %s resolver returned null", providerName)
	}
	defer C.dd_dll_free((*C.char)(unsafe.Pointer(ptr)))

	hostname := C.GoString((*C.char)(unsafe.Pointer(ptr)))
	if hostname == "" {
		return "", fmt.Errorf("dll %s resolver returned empty hostname", providerName)
	}

	log.Infof("Resolved a hostname thorugh the DLL (provider: %s): %s", providerName, hostname)
	return hostname, nil
}

// fromDLLOS attempts to resolve the OS hostname via the external DLL implementation.
func fromDLLOS(ctx context.Context, currentHostname string) (string, error) {
	return dllResolveHostname("os", "")
}

// fromDLLFQDN attempts to resolve the FQDN via the external DLL implementation.
func fromDLLFQDN(ctx context.Context, currentHostname string) (string, error) {
	if !osHostnameUsable(ctx) {
		return "", fmt.Errorf("FQDN hostname is not usable")
	}
	if !pkgconfigsetup.Datadog().GetBool("hostname_fqdn") {
		return "", fmt.Errorf("'hostname_fqdn' configuration is not enabled")
	}

	return dllResolveHostname("fqdn", "")
}

// getDLLOSProvider returns the providers backed by the DLL resolver when enabled.
func getDLLOSProvider() provider {
	return provider{
		name:             "dll_os",
		cb:               fromDLLOS,
		stopIfSuccessful: false,
		expvarName:       "dll_os",
	}
}

// getDLLFQDNProvider returns the DLL-backed FQDN provider to be placed alongside the Go FQDN provider.
func getDLLFQDNProvider() provider {
	return provider{
		name:             "dll_fqdn",
		cb:               fromDLLFQDN,
		stopIfSuccessful: false,
		expvarName:       "dll_fqdn",
	}
}
