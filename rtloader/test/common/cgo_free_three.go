// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcommon

/*
#cgo !windows LDFLAGS: -ldl

// c_callCgoFree calls the cgo_free from the dynamically loaded `three` library.

#ifdef _WIN32
#include <windows.h>
#include <stdio.h>

void c_callCgoFree(void *ptr) {
    HMODULE handle = GetModuleHandleA("libdatadog-agent-three.dll");
    if (handle == NULL) {
        fprintf(stderr, "c_callCgoFree: libdatadog-agent-three.dll is not loaded\n");
        return;
    }
    typedef void (*cgo_free_t)(void *);
    cgo_free_t fn = (cgo_free_t)GetProcAddress(handle, "cgo_free");
    if (fn == NULL) {
        fprintf(stderr, "c_callCgoFree: cgo_free symbol not found in libdatadog-agent-three.dll\n");
        return;
    }
    fn(ptr);
}

#else
#include <dlfcn.h>
#include <stdio.h>

#if defined(__linux__) || defined(__FreeBSD__)
#  define THREE_LIB "libdatadog-agent-three.so"
#elif defined(__APPLE__)
#  define THREE_LIB "libdatadog-agent-three.dylib"
#endif

void c_callCgoFree(void *ptr) {
    void *handle = dlopen(THREE_LIB, RTLD_NOLOAD | RTLD_LAZY);
    if (handle == NULL) {
        fprintf(stderr, "c_callCgoFree: " THREE_LIB " is not loaded\n");
        return;
    }
    typedef void (*cgo_free_t)(void *);
    cgo_free_t fn = (cgo_free_t)dlsym(handle, "cgo_free");
    if (fn == NULL) {
        fprintf(stderr, "c_callCgoFree: cgo_free symbol not found in " THREE_LIB "\n");
        dlclose(handle);
        return;
    }
    fn(ptr);
    dlclose(handle);
}

#endif
*/
import "C"
