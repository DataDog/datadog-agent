#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <dlfcn.h>

static volatile int running = 1;
static void* lib_handle = NULL;

// Function pointer for the Go shared library main function
typedef int (*run_system_probe_t)(void);
static run_system_probe_t run_system_probe_fn = NULL;

void signal_handler(int sig) {
    printf("Received signal %d, shutting down...\n", sig);
    running = 0;
}

int load_system_probe_library(const char* lib_path) {
    // Clear any existing errors
    dlerror();
    
    // Load the shared library
    lib_handle = dlopen(lib_path, RTLD_LAZY);
    if (!lib_handle) {
        fprintf(stderr, "Cannot load library %s: %s\n", lib_path, dlerror());
        return -1;
    }
    
    // Get the main function pointer
    run_system_probe_fn = (run_system_probe_t) dlsym(lib_handle, "RunSystemProbe");
    if (!run_system_probe_fn) {
        fprintf(stderr, "Cannot find RunSystemProbe function: %s\n", dlerror());
        dlclose(lib_handle);
        return -1;
    }
    
    return 0;
}

void cleanup_library(void) {
    if (lib_handle) {
        dlclose(lib_handle);
        lib_handle = NULL;
    }
}

int main(int argc, char* argv[]) {
    const char* enable_env = getenv("DD_SYSTEM_PROBE_ENABLED");
    
    // Check if system probe should be enabled
    if (enable_env && (strcmp(enable_env, "1") == 0 || strcasecmp(enable_env, "true") == 0)) {
        printf("DD_SYSTEM_PROBE_ENABLED is set, loading full system-probe...\n");
        
        // Try to find the shared library in the same directory as the binary
        char lib_path[1024];
        char* binary_path = argv[0];
        char* last_slash = strrchr(binary_path, '/');
        
        if (last_slash) {
            size_t dir_len = last_slash - binary_path + 1;
            strncpy(lib_path, binary_path, dir_len);
            lib_path[dir_len] = '\0';
            strcat(lib_path, "libsystemprobe.so");
        } else {
            strcpy(lib_path, "./libsystemprobe.so");
        }
        
        // Load the system probe library
        if (load_system_probe_library(lib_path) != 0) {
            fprintf(stderr, "Failed to load system probe library\n");
            return 1;
        }
        
        // Call the main system probe function (it will handle all arguments)
        int result = run_system_probe_fn();
        
        cleanup_library();
        return result;
        
    } else {
        // Check for help flag in lightweight mode
        for (int i = 1; i < argc; i++) {
            if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
                printf("Datadog Agent System Probe (Lightweight Mode)\n");
                printf("Usage: %s [options]\n", argv[0]);
                printf("Options:\n");
                printf("  -h, --help               show this help message\n");
                printf("\nEnvironment Variables:\n");
                printf("  DD_SYSTEM_PROBE_ENABLED  set to '1' or 'true' to enable full system-probe\n");
                printf("                          (default: lightweight mode)\n");
                printf("\nTo see full system-probe options, run:\n");
                printf("  DD_SYSTEM_PROBE_ENABLED=1 %s --help\n", argv[0]);
                return 0;
            }
        }
        
        // Set up signal handlers for lightweight mode
        signal(SIGINT, signal_handler);
        signal(SIGTERM, signal_handler);
        signal(SIGPIPE, SIG_IGN);
        
        printf("DD_SYSTEM_PROBE_ENABLED not set, running in lightweight mode\n");
        printf("System probe is sleeping. Set DD_SYSTEM_PROBE_ENABLED=1 to enable full functionality.\n");
        
        // Sleep indefinitely in lightweight mode
        while (running) {
            sleep(60); // Sleep for 60 seconds at a time
        }
        
        printf("Lightweight system probe stopped\n");
        return 0;
    }
}