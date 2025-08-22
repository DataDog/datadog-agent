#ifndef USM_RS_H
#define USM_RS_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// C-compatible service metadata structure
typedef struct {
    char* name;
    char* source;
    char* dd_service;
    int dd_service_injected;
    char** additional_names;
    int additional_names_len;
} CServiceMetadata;

// Language constants
#define USM_LANG_UNKNOWN 0
#define USM_LANG_JAVA 1
#define USM_LANG_PYTHON 2
#define USM_LANG_NODE 3
#define USM_LANG_PHP 4
#define USM_LANG_RUBY 5
#define USM_LANG_DOTNET 6
#define USM_LANG_GO 7
#define USM_LANG_RUST 8
#define USM_LANG_CPP 9

/**
 * Extract service metadata from process information
 * 
 * @param language Language identifier (use USM_LANG_* constants)
 * @param pid Process ID
 * @param args Array of command line arguments (null-terminated strings)
 * @param args_len Number of arguments
 * @param envs Array of environment variables (key-value pairs, null-terminated strings)
 * @param envs_len Number of environment variables (should be even)
 * @return Pointer to CServiceMetadata or NULL on error
 */
CServiceMetadata* usm_extract_service_metadata(
    int language,
    unsigned int pid,
    const char* const* args,
    int args_len,
    const char* const* envs,
    int envs_len
);

/**
 * Free service metadata structure
 * 
 * @param metadata Pointer returned by usm_extract_service_metadata
 */
void usm_free_service_metadata(CServiceMetadata* metadata);

/**
 * Get the library version
 * 
 * @return Version string (static, do not free)
 */
const char* usm_version(void);

#ifdef __cplusplus
}
#endif

#endif // USM_RS_H