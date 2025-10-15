#include "ffi.h"

#include <stdlib.h>
#include <stdio.h>
#include <string.h>

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
		goto done;
    }

    // get symbol pointers of 'Run' and 'Free' functions
    lib_handles.run = (run_function_t *)GetProcAddress(lib_handles.lib, "Run");
    if (!lib_handles.run) {
        char error_msg[256];
        int error_code = GetLastError();
        snprintf(error_msg, sizeof(error_msg), "unable to get shared library 'Run' symbol, error code: %d", error_code);
		*error = strdup(error_msg);
		goto done;
    }

done:
	return lib_handles;
}

void close_shared_library(void *lib_handle, const char **error) {
	// verify pointer
	if (!lib_handle) {
		// TODO: goto
	}
    
	FreeLibrary(lib_handle);
}

#else

handles_t load_shared_library(const char *lib_path, const char **error) {
	handles_t lib_handles;

    // load the library
    char *dlsym_error = NULL;

    lib_handles.lib = dlopen(lib_path, RTLD_LAZY | RTLD_GLOBAL);
    dlsym_error = dlerror();
    if (dlsym_error) {
		*error = strdup(dlsym_error);
		goto done;
    }

    // get symbol pointer of 'Run' function
    lib_handles.run = (run_function_t *)dlsym(lib_handles.lib, "Run");
    dlsym_error = dlerror();
    if (dlsym_error) {
		dlclose(lib_handles.lib);
		*error = strdup(dlsym_error);
		goto done;
    }

done:
	return lib_handles;
}

void close_shared_library(void *lib_handle, const char **error) {
    // verify pointer
	if (!lib_handle) {
		*error = strdup("pointer to shared library is NULL");
	} else {
        dlclose(lib_handle);
        
        // check for closing errors
        char *dlsym_error = dlerror();
        if (dlsym_error) {
            *error = strdup(dlsym_error);
        }
    }
}
#endif

void run_shared_library(run_function_t *run_handle, char *check_id, char *init_config, char *instance_config, aggregator_t *aggregator, const char **error) {
    // verify pointers
    if (!run_handle) {
        *error = strdup("pointer to shared library 'Run' symbol is NULL");
    } else {
        // run the shared library check and return any error has occurred
        (run_handle)(check_id, init_config, instance_config, aggregator, error);
    }
}
