#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <dlfcn.h>

extern char **environ;

static volatile int running = 1;
static void* lib_handle = NULL;

// Function pointers for the Go shared libraries
typedef int (*run_system_probe_t)(void);
typedef int (*run_service_discovery_t)(void);

static run_system_probe_t run_system_probe_fn = NULL;
static run_service_discovery_t run_service_discovery_fn = NULL;

void signal_handler(int sig) {
    printf("Received signal %d, shutting down...\n", sig);
    running = 0;
}

int load_library(const char* lib_path, const char* function_name, void** function_ptr) {
    // Clear any existing errors
    dlerror();
    
    // Load the shared library
    lib_handle = dlopen(lib_path, RTLD_LAZY);
    if (!lib_handle) {
        fprintf(stderr, "Cannot load library %s: %s\n", lib_path, dlerror());
        return -1;
    }
    
    // Get the function pointer
    *function_ptr = dlsym(lib_handle, function_name);
    if (!*function_ptr) {
        fprintf(stderr, "Cannot find %s function: %s\n", function_name, dlerror());
        dlclose(lib_handle);
        return -1;
    }
    
    return 0;
}

int check_system_probe_env_vars(char** env_vars) {
    char** env = env_vars ? env_vars : environ;
    
    // Check for any DD_SYSTEM_PROBE_* environment variable
    for (int i = 0; env[i] != NULL; i++) {
        if (strncmp(env[i], "DD_SYSTEM_PROBE_", 16) == 0) {
            // Found a DD_SYSTEM_PROBE_* variable, check if it has a non-empty value
            char* eq_pos = strchr(env[i], '=');
            if (eq_pos && eq_pos[1] != '\0') {
                printf("Found system-probe environment variable: %.*s\n", 
                       (int)(strchr(env[i], '=') - env[i]), env[i]);
                return 1;
            }
        }
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
    // Check for help flag first
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
            // Determine which help to show based on environment
            if (check_system_probe_env_vars(NULL)) {
                // System probe environment detected, load system-probe library
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
                
                if (load_library(lib_path, "RunSystemProbe", (void**)&run_system_probe_fn) == 0) {
                    int result = run_system_probe_fn();
                    cleanup_library();
                    return result;
                } else {
                    fprintf(stderr, "Failed to load system probe library for help\n");
                    return 1;
                }
            } else {
                // No system probe env vars, show service discovery help  
                printf("Datadog Service Discovery (Lightweight)\n");
                printf("Usage: %s [options]\n", argv[0]);
                printf("Options:\n");
                printf("  -h, --help               show this help message\n");
                printf("  -socket PATH             Unix socket path (default: /opt/datadog-agent/run/service-discovery.sock)\n");
                printf("  -config PATH             Path to configuration file\n");
                printf("\nEnvironment Variables:\n");
                printf("  DD_SYSTEM_PROBE_*        any DD_SYSTEM_PROBE_ variable enables full system-probe\n");
                printf("                          (default: service-discovery mode)\n");
                printf("\nTo see full system-probe options, set a DD_SYSTEM_PROBE_ variable:\n");
                printf("  DD_SYSTEM_PROBE_ENABLED=1 %s --help\n", argv[0]);
                return 0;
            }
        }
    }
    
    // Determine which mode to run based on environment variables
    if (check_system_probe_env_vars(NULL)) {
        printf("System-probe environment detected, loading full system-probe...\n");
        
        // Try to find the system-probe shared library
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
        if (load_library(lib_path, "RunSystemProbe", (void**)&run_system_probe_fn) != 0) {
            fprintf(stderr, "Failed to load system probe library\n");
            return 1;
        }
        
        // Call the main system probe function
        int result = run_system_probe_fn();
        
        cleanup_library();
        return result;
        
    } else {
        printf("No system-probe environment variables detected, running service-discovery...\n");
        
        // Try to find the service-discovery shared library
        char lib_path[1024];
        char* binary_path = argv[0];
        char* last_slash = strrchr(binary_path, '/');
        
        if (last_slash) {
            size_t dir_len = last_slash - binary_path + 1;
            strncpy(lib_path, binary_path, dir_len);
            lib_path[dir_len] = '\0';
            strcat(lib_path, "libservicediscovery.so");
        } else {
            strcpy(lib_path, "./libservicediscovery.so");
        }
        
        // Load the service discovery library
        if (load_library(lib_path, "RunServiceDiscovery", (void**)&run_service_discovery_fn) != 0) {
            fprintf(stderr, "Failed to load service discovery library\n");
            return 1;
        }
        
        // Call the main service discovery function
        int result = run_service_discovery_fn();
        
        cleanup_library();
        return result;
    }
}