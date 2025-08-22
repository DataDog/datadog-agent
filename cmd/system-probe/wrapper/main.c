#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <dlfcn.h>
#include <errno.h>
#include <sys/wait.h>

static volatile int running = 1;
static void* lib_handle = NULL;

// Function pointers for the Go shared library
typedef int (*start_system_probe_t)(char*, char*, char*);
typedef int (*stop_system_probe_t)(void);
typedef int (*wait_for_system_probe_t)(void);

static start_system_probe_t start_system_probe_fn = NULL;
static stop_system_probe_t stop_system_probe_fn = NULL;
static wait_for_system_probe_t wait_for_system_probe_fn = NULL;

void signal_handler(int sig) {
    printf("Received signal %d, shutting down...\n", sig);
    running = 0;
    
    if (stop_system_probe_fn != NULL) {
        stop_system_probe_fn();
    }
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
    
    // Get function pointers
    start_system_probe_fn = (start_system_probe_t) dlsym(lib_handle, "StartSystemProbe");
    if (!start_system_probe_fn) {
        fprintf(stderr, "Cannot find StartSystemProbe function: %s\n", dlerror());
        dlclose(lib_handle);
        return -1;
    }
    
    stop_system_probe_fn = (stop_system_probe_t) dlsym(lib_handle, "StopSystemProbe");
    if (!stop_system_probe_fn) {
        fprintf(stderr, "Cannot find StopSystemProbe function: %s\n", dlerror());
        dlclose(lib_handle);
        return -1;
    }
    
    wait_for_system_probe_fn = (wait_for_system_probe_t) dlsym(lib_handle, "WaitForSystemProbe");
    if (!wait_for_system_probe_fn) {
        fprintf(stderr, "Cannot find WaitForSystemProbe function: %s\n", dlerror());
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
    const char* config_path = NULL;
    const char* fleet_policies_dir = NULL;
    const char* pid_file = NULL;
    
    // Parse command line arguments
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-c") == 0 || strcmp(argv[i], "--config") == 0) {
            if (i + 1 < argc) {
                config_path = argv[++i];
            }
        } else if (strcmp(argv[i], "--fleetcfgpath") == 0) {
            if (i + 1 < argc) {
                fleet_policies_dir = argv[++i];
            }
        } else if (strcmp(argv[i], "-p") == 0 || strcmp(argv[i], "--pid") == 0) {
            if (i + 1 < argc) {
                pid_file = argv[++i];
            }
        } else if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
            printf("Datadog Agent System Probe\n");
            printf("Usage: %s [options]\n", argv[0]);
            printf("Options:\n");
            printf("  -c, --config PATH        path to directory containing system-probe.yaml\n");
            printf("  --fleetcfgpath PATH      path to the directory containing fleet policies\n");
            printf("  -p, --pid PATH           path to the pidfile\n");
            printf("  -h, --help               show this help message\n");
            printf("\nEnvironment Variables:\n");
            printf("  DD_SYSTEM_PROBE_ENABLED  set to '1' or 'true' to enable full system-probe\n");
            printf("                          (default: lightweight mode)\n");
            return 0;
        }
    }
    
    // Set up signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
    signal(SIGPIPE, SIG_IGN); // Ignore SIGPIPE as per original code
    
    printf("Datadog System Probe Wrapper starting...\n");
    
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
        
        // Start the system probe
        int result = start_system_probe_fn((char*)config_path, (char*)fleet_policies_dir, (char*)pid_file);
        if (result != 0) {
            fprintf(stderr, "Failed to start system probe\n");
            cleanup_library();
            return 1;
        }
        
        printf("System probe started successfully\n");
        
        // Wait for the system probe to finish
        result = wait_for_system_probe_fn();
        
        printf("System probe stopped\n");
        cleanup_library();
        return result;
        
    } else {
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