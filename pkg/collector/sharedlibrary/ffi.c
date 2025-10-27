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

    // get symbol pointers of 'Run' and 'Free' functions
    lib_handles.run = (run_function_t *)GetProcAddress(lib_handles.lib, "Run");
    if (!lib_handles.run) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Run' symbol, error code: %d", error_code);
		*error = strdup(error_msg);
		return lib_handles;
    }

    return lib_handles;
}

void close_shared_library(void *lib_handle, const char **error) {
	// verify pointer
	if (!lib_handle) {
		*error = strdup("pointer to shared library is NULL");
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

    // get symbol pointer of 'Run' function
    lib_handles.run = (run_function_t *)dlsym(lib_handles.lib, "Run");
    
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

void run_shared_library(run_function_t *run_handle, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator, const char **error) {
    // verify `Run` handle
    if (!run_handle) {
        *error = strdup("pointer to 'Run' symbol of the shared library is NULL");
    }

    // run the shared library check and put any errors string in the `error` variable
    (run_handle)(check_id, init_config, instance_config, aggregator, error);
}
