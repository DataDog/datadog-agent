#include "ffi.h"

#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#ifdef _WIN32
#    include <Windows.h>
#else
#    include <dlfcn.h>
#endif

#ifdef _WIN32
handles_t load_shared_library(const char *lib_path, const char **error) {
	handles_t lib_handles;

    // load the library
    lib_handles.lib = LoadLibraryA(lib_path);
    if (!lib_handles.lib) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to open shared library, error code: %d", error_code);
		*error = strdup(error_msg);
		return lib_handles;
    }

    // get pointer of 'Run' symbol
    lib_handles.run = (run_function_t *)GetProcAddress(lib_handles.lib, "Run");
    if (!lib_handles.run) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Run' symbol, error code: %d", error_code);
		*error = strdup(error_msg);
		return lib_handles;
    }

    // get pointer of 'Version' symbol
    lib_handles.version = (version_function_t *)GetProcAddress(lib_handles.lib, "Version");
    if (!lib_handles.version) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Version' symbol, error code: %d", error_code);
		*error = strdup(error_msg);
		return lib_handles;
    }

    return lib_handles;
}

void close_shared_library(void *lib_handle, const char **error) {
	// verify pointer
	if (!lib_handle) {
		*error = strdup("pointer to shared library is NULL");
        return;
	}

    // close the library and check for errors (error_code == 0)
    int error_code = FreeLibrary(lib_handle);
    if (!error_code) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to close shared library, error code: %d", error_code);
		*error = strdup(error_msg);
    }
}

#else

handles_t load_shared_library(const char *lib_path, const char **error) {
    handles_t lib_handles;
    char *dlsym_error = NULL;

    // load the library
    lib_handles.lib = dlopen(lib_path, RTLD_LAZY | RTLD_GLOBAL);
    
    // catch library opening error
    dlsym_error = dlerror();
    if (dlsym_error) {
		*error = strdup(dlsym_error);
		return lib_handles;
    }

    // get pointer of 'Run' symbol
    lib_handles.run = (run_function_t *)dlsym(lib_handles.lib, "Run");
    
    // catch symbol errors and close the library if there are any
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handles.lib);
		*error = strdup(dlsym_error);
		return lib_handles;
    }

    // get pointer of 'Version' symbol
    lib_handles.version = (version_function_t *)dlsym(lib_handles.lib, "Version");
    
    // catch symbol errors and close the library if there are any
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handles.lib);
		*error = strdup(dlsym_error);
		return lib_handles;
    }

    return lib_handles;
}

void close_shared_library(void *lib_handle, const char **error) {
    // verify library handle
	if (!lib_handle) {
        *error = strdup("pointer to shared library is NULL");
        return;
	}

    // close the library
    dlclose(lib_handle);

    // check for closing errors
    char *dlsym_error = dlerror();
    if (dlsym_error) {
        *error = strdup(dlsym_error);
    }
}
#endif

void run_shared_library(run_function_t *run_ptr, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator, const char **error) {
    // verify `Run` pointer
    if (!run_ptr) {
        *error = strdup("pointer to 'Run' symbol of the shared library is NULL");
        return;
    }

    // run the shared library check and put any errors string in the `error` variable
    (run_ptr)(check_id, init_config, instance_config, aggregator, error);
}

const char *get_version_shared_library(version_function_t *version_ptr, const char **error) {
    // verify `Version` pointer
    if (!version_ptr) {
        *error = strdup("pointer to 'Version' symbol of the shared library is NULL");
        return NULL;
    }

    // retrieve the version of the shared library check and put any errors string in the `error` variable
    return (version_ptr)(error);
}
