// Unless explicitly stated otherwise all files in this repository are
// dual-licensed under the Apache-2.0 License or BSD-3-Clause License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

#ifndef pw_h
#define pw_h

#ifdef __cplusplus
extern "C"
{
#endif

#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>

#define DDWAF_MAX_STRING_LENGTH 4096
#define DDWAF_MAX_CONTAINER_DEPTH 20
#define DDWAF_MAX_CONTAINER_SIZE 256
#define DDWAF_RUN_TIMEOUT 5000

/**
 * @enum DDWAF_OBJ_TYPE
 *
 * Specifies the type of a ddwaf::object.
 **/
typedef enum
{
    DDWAF_OBJ_INVALID     = 0,
    /** Value shall be decoded as a int64_t (or int32_t on 32bits platforms). **/
    DDWAF_OBJ_SIGNED   = 1 << 0,
    /** Value shall be decoded as a uint64_t (or uint32_t on 32bits platforms). **/
    DDWAF_OBJ_UNSIGNED = 1 << 1,
    /** Value shall be decoded as a UTF-8 string of length nbEntries. **/
    DDWAF_OBJ_STRING   = 1 << 2,
    /** Value shall be decoded as an array of ddwaf_object of length nbEntries, each item having no parameterName. **/
    DDWAF_OBJ_ARRAY    = 1 << 3,
    /** Value shall be decoded as an array of ddwaf_object of length nbEntries, each item having a parameterName. **/
    DDWAF_OBJ_MAP      = 1 << 4,
} DDWAF_OBJ_TYPE;

/**
 * @enum DDWAF_RET_CODE
 *
 * Codes returned by ddwaf_run.
 **/
typedef enum
{
    DDWAF_ERR_INTERNAL     = -3,
    DDWAF_ERR_INVALID_OBJECT = -2,
    DDWAF_ERR_INVALID_ARGUMENT = -1,
    DDWAF_GOOD             = 0,
    DDWAF_MONITOR          = 1,
    DDWAF_BLOCK            = 2
} DDWAF_RET_CODE;

/**
 * @enum DDWAF_LOG_LEVEL
 *
 * Internal WAF log levels, to be used when setting the minimum log level and cb.
 **/
typedef enum
{
    DDWAF_LOG_TRACE,
    DDWAF_LOG_DEBUG,
    DDWAF_LOG_INFO,
    DDWAF_LOG_WARN,
    DDWAF_LOG_ERROR,
    DDWAF_LOG_OFF,
} DDWAF_LOG_LEVEL;

#ifdef __cplusplus
class PowerWAF;
class PWAdditive;
using ddwaf_handle = PowerWAF *;
using ddwaf_context = PWAdditive *;
#else
typedef struct _ddwaf_handle* ddwaf_handle;
typedef struct _ddwaf_context* ddwaf_context;
#endif

typedef struct _ddwaf_object ddwaf_object;
typedef struct _ddwaf_config ddwaf_config;
typedef struct _ddwaf_result ddwaf_result;
typedef struct _ddwaf_version ddwaf_version;
typedef struct _ddwaf_ruleset_info ddwaf_ruleset_info;
/**
 * @struct ddwaf_object
 *
 * Generic object used to pass data and rules to the WAF.
 **/
struct _ddwaf_object
{
    const char* parameterName;
    uint64_t parameterNameLength;
    // uintValue should be at least as wide as the widest type on the platform.
    union
    {
        const char* stringValue;
        uint64_t uintValue;
        int64_t intValue;
        ddwaf_object* array;
    };
    uint64_t nbEntries;
    DDWAF_OBJ_TYPE type;
};

/**
 * @struct ddwaf_config
 *
 * Configuration to be provided to the WAF
 **/
struct _ddwaf_config
{
    struct {
        /** Maximum size of ddwaf::object containers. */
        uint32_t max_container_size;
        /** Maximum depth of ddwaf::object containers. */
        uint32_t max_container_depth;
        /** Maximum length of ddwaf::object strings. */
        uint32_t max_string_length;
    } limits;

    /** Obfuscator regexes - the strings are owned by the caller */
    struct {
        /** Regular expression for key-based obfuscation */
        const char *key_regex;
        /** Regular expression for value-based obfuscation */
        const char *value_regex;
    } obfuscator;
};

/**
 * @struct ddwaf_result
 *
 * Structure containing the result of a WAF run.
 **/
struct _ddwaf_result
{
    /** Whether there has been a timeout during the operation **/
    bool timeout;
    /** Run result in JSON format **/
    const char* data;
    /** Total WAF runtime in nanoseconds **/
    uint64_t total_runtime;
};

/**
 * @ddwaf_version
 *
 * Structure containing the version of the WAF following semver.
 **/
struct _ddwaf_version
{
    uint16_t major;
    uint16_t minor;
    uint16_t patch;
};

/**
 * @ddwaf_ruleset_info
 *
 * Structure containing diagnostics on the provided ruleset.
 * */
struct _ddwaf_ruleset_info
{
    /** Number of rules successfully loaded **/
    uint16_t loaded;
    /** Number of rules which failed to parse **/
    uint16_t failed;
    /** Map from an error string to an array of all the rule ids for which
     *  that error was raised. {error: [rule_ids]} **/
    ddwaf_object errors;
    /** Ruleset version **/
    const char *version;
};

/**
 * @typedef ddwaf_object_free_fn
 *
 * Type of the function to free ddwaf::objects.
 **/
typedef void (*ddwaf_object_free_fn)(ddwaf_object *object);

/**
 * @typedef ddwaf_log_cb
 *
 * Callback that powerwaf will call to relay messages to the binding.
 *
 * @param level The logging level.
 * @param function The native function that emitted the message. (nonnull)
 * @param file The file of the native function that emmitted the message. (nonnull)
 * @param line The line where the message was emmitted.
 * @param message The size of the logging message. NUL-terminated
 * @param message_len The length of the logging message (excluding NUL terminator).
 */
typedef void (*ddwaf_log_cb)(
    DDWAF_LOG_LEVEL level, const char* function, const char* file, unsigned line,
    const char* message, uint64_t message_len);

/**
 * ddwaf_init
 *
 * Initialize a ddwaf instance
 *
 * @param rule ddwaf::object containing the patterns to be used by the WAF. (nonnull)
 * @param config Optional configuration of the WAF. (nullable)
 * @param info Optional ruleset parsing diagnostics. (nullable)
 *
 * @return Handle to the WAF instance.
 **/
ddwaf_handle ddwaf_init(const ddwaf_object *rule,
    const ddwaf_config* config, ddwaf_ruleset_info *info);

/**
 * ddwaf_destroy
 *
 * Destroy a WAF instance.
 *
 * @param Handle to the WAF instance.
 */
void ddwaf_destroy(ddwaf_handle handle);
/**
 * ddwaf_ruleset_info_free
 *
 * Free the memory associated with the ruleset info structure.
 *
 * @param info Ruleset info to free.
 * */
void ddwaf_ruleset_info_free(ddwaf_ruleset_info *info);
/**
 * ddwaf_required_addresses
 *
 * Get a list of required (root) addresses. The memory is owned by the WAF and
 * should not be freed.
 *
 * @param Handle to the WAF instance.
 * @param size Output parameter in which the size will be returned. The value of
 *             size will be 0 if the return value is nullptr.
 * @return NULL if error, otherwise a pointer to an array with size elements.
 **/
const char* const* ddwaf_required_addresses(const ddwaf_handle handle, uint32_t *size);
/**
 * ddwaf_context_init
 *
 * Context object to perform matching using the provided WAF instance.
 *
 * @param handle Handle of the WAF instance containing the ruleset definition. (nonnull)
 * @param obj_free Function to free the ddwaf::object provided to the context
 *                 during calls to ddwaf_run. If the value of this function is
 *                 NULL, the objects will not be freed. By default the value of
 *                 this parameter should be ddwaf_object_free.
 *
 * @return Handle to the context instance.
 *
 * @note The WAF instance needs to be valid for the lifetime of the context.
 **/
ddwaf_context ddwaf_context_init(const ddwaf_handle handle, ddwaf_object_free_fn obj_free);

/**
 * ddwaf_run
 *
 * Perform a matching operation on the provided data
 *
 * @param context WAF context to be used in this run, this will determine the
 *                ruleset which will be used and it will also ensure that
 *                parameters are taken into account across runs (nonnull)
 * @param data Data on which to perform the pattern matching. This data will be
 *             stored by the context and used across multiple calls to this
 *             function. Once the context is destroyed, the used-defined free
 *             function will be used to free the data provided. Note that the
 *             data passed must be valid until the destruction of the context.
 *             (nonull)
 * @param result Structure containing the result of the operation. (nullable)
 * @param timeout Maximum time budget in microseconds.
 *
 * @return Return code of the operation, also contained in the result structure.
 * @error DDWAF_ERR_INVALID_ARGUMENT The context is invalid, the data will not
 *                                   be freed.
 * @error DDWAF_ERR_INVALID_OBJECT The data provided didn't match the desired
 *                                 structure or contained invalid objects, the
 *                                 data will be freed by this function.
 * @error DDWAF_ERR_TIMEOUT The operation timed out, the data will be owned by
 *                          the context and freed during destruction.
 * @error DDWAF_ERR_INTERNAL There was an unexpected error and the operation did
 *                           not succeed. The state of the WAF is undefined if
 *                           this error is produced and the ownership of the
 *                           data is unknown. The result structure will not be
 *                           filled if this error occurs.
 **/
DDWAF_RET_CODE ddwaf_run(ddwaf_context context, ddwaf_object *data,
                         ddwaf_result *result,  uint64_t timeout);

/**
 * ddwaf_context_destroy
 *
 * Performs the destruction of the context, freeing the data passed to it through
 * ddwaf_run using the used-defined free function.
 *
 * @param context Context to destroy. (nonnull)
 **/
void ddwaf_context_destroy(ddwaf_context context);

/**
 * ddwaf_result_free
 *
 * Free a ddwaf_result structure.
 *
 * @param result Structure to free. (nonnull)
 **/
void ddwaf_result_free(ddwaf_result *result);

/**
 * ddwaf_object_invalid
 *
 * Creates an invalid object.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_invalid(ddwaf_object *object);

/**
 * ddwaf_object_string
 *
 * Creates an object from a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String to initialise the object with, this string will be copied
 *               and its length will be calculated using strlen(string). (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_string(ddwaf_object *object, const char *string);

/**
 * ddwaf_object_stringl
 *
 * Creates an object from a string and its length.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String to initialise the object with, this string will be
 *               copied. (nonnull)
 * @param length Length of the string.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_stringl(ddwaf_object *object, const char *string, size_t length);

/**
 * ddwaf_object_stringl_nc
 *
 * Creates an object with the string pointer and length provided.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param string String pointer to initialise the object with.
 * @param length Length of the string.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_stringl_nc(ddwaf_object *object, const char *string, size_t length);

/**
 * ddwaf_object_unsigned
 *
 * Creates an object using an unsigned integer (64-bit). The resulting object
 * will contain a string created using the integer provided. This is the
 * preferred method for passing an unsigned integer to the WAF.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_unsigned(ddwaf_object *object, uint64_t value);

/**
 * ddwaf_object_signed
 *
 * Creates an object using a signed integer (64-bit). The resulting object
 * will contain a string created using the integer provided. This is the
 * preferred method for passing a signed integer to the WAF.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_signed(ddwaf_object *object, int64_t value);

/**
 * ddwaf_object_unsigned_force
 *
 * Creates an object using an unsigned integer (64-bit). The resulting object
 * will contain an unsigned integer as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_unsigned_force(ddwaf_object *object, uint64_t value);

/**
 * ddwaf_object_signed_force
 *
 * Creates an object using a signed integer (64-bit). The resulting object
 * will contain a signed integer as opposed to a string.
 *
 * @param object Object to perform the operation on. (nonnull)
 * @param value Integer to initialise the object with.
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_signed_force(ddwaf_object *object, int64_t value);

/**
 * ddwaf_object_array
 *
 * Creates an array object, for sequential storage.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_array(ddwaf_object *object);

/**
 * ddwaf_object_map
 *
 * Creates a map object, for key-value storage.
 *
 * @param object Object to perform the operation on. (nonnull)
 *
 * @return A pointer to the passed object or NULL if the operation failed.
 **/
ddwaf_object* ddwaf_object_map(ddwaf_object *object);

/**
 * ddwaf_object_array_add
 *
 * Inserts an object into an array object.
 *
 * @param array Array in which to insert the object. (nonnull)
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_array_add(ddwaf_object *array, ddwaf_object *object);

/**
 * ddwaf_object_map_add
 *
 * Inserts an object into an map object, using a key.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied and its
 *            length will be calcualted using strlen(key). (nonnull)
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_add(ddwaf_object *map, const char *key, ddwaf_object *object);

/**
 * ddwaf_object_map_addl
 *
 * Inserts an object into an map object, using a key and its length.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied (nonnull)
 * @param length Length of the key.
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_addl(ddwaf_object *map, const char *key, size_t length, ddwaf_object *object);

/**
 * ddwaf_object_map_addl_nc
 *
 * Inserts an object into an map object, using a key and its length, but without
 * creating a copy of the key.
 *
 * @param map Map in which to insert the object. (nonnull)
 * @param key The key for indexing purposes, this string will be copied (nonnull)
 * @param length Length of the key.
 * @param object Object to insert into the array. (nonnull)
 *
 * @return The success or failure of the operation.
 **/
bool ddwaf_object_map_addl_nc(ddwaf_object *map, const char *key, size_t length, ddwaf_object *object);

/**
 * ddwaf_object_type
 *
 * Returns the type of the object.
 *
 * @param object The object from which to get the type.
 *
 * @return The object type of DDWAF_OBJ_INVALID if NULL.
 **/
DDWAF_OBJ_TYPE ddwaf_object_type(ddwaf_object *object);

/**
 * ddwaf_object_size
 *
 * Returns the size of the container object.
 *
 * @param object The object from which to get the size.
 *
 * @return The object size or 0 if the object is not a container (array, map).
 **/
size_t ddwaf_object_size(ddwaf_object *object);

/**
 * ddwaf_object_length
 *
 * Returns the length of the string object.
 *
 * @param object The object from which to get the length.
 *
 * @return The string length or 0 if the object is not a string.
 **/
size_t ddwaf_object_length(ddwaf_object *object);

/**
 * ddwaf_object_get_key
 *
 * Returns the key contained within the object.
 *
 * @param object The object from which to get the key.
 * @param length Output parameter on which to return the length of the key,
 *               this parameter is optional / nullable.
 *
 * @return The key of the object or NULL if the object doesn't contain a key.
 **/
const char* ddwaf_object_get_key(ddwaf_object *object, size_t *length);

/**
 * ddwaf_object_get_string
 *
 * Returns the string contained within the object.
 *
 * @param object The object from which to get the string.
 * @param length Output parameter on which to return the length of the string,
 *               this parameter is optional / nullable.
 *
 * @return The string of the object or NULL if the object is not a string.
 **/
const char* ddwaf_object_get_string(ddwaf_object *object, size_t *length);

/**
 * ddwaf_object_get_unsigned
 *
 * Returns the uint64 contained within the object.
 *
 * @param object The object from which to get the integer.
 *
 * @return The integer or 0 if the object is not an unsigned.
 **/
uint64_t ddwaf_object_get_unsigned(ddwaf_object *object);

/**
 * ddwaf_object_get_signed
 *
 * Returns the int64 contained within the object.
 *
 * @param object The object from which to get the integer.
 *
 * @return The integer or 0 if the object is not a signed.
 **/
int64_t ddwaf_object_get_signed(ddwaf_object *object);

/**
 * ddwaf_object_get_index
 *
 * Returns the object contained in the container at the given index.
 *
 * @param object The container from which to extract the object.
 * @param index The position of the required object within the container.
 *
 * @return The requested object or NULL if the index is out of bounds or the
 *         object is not a container.
 **/
ddwaf_object* ddwaf_object_get_index(ddwaf_object *object, size_t index);


/**
 * ddwaf_object_free
 *
 * @param object Object to free. (nonnull)
 **/
void ddwaf_object_free(ddwaf_object *object);

/**
 * ddwaf_get_version
 *
 * Return the version of the library
 *
 * @param version Version structure following semver
 **/
void ddwaf_get_version(ddwaf_version *version);

/**
 * ddwaf_set_log_cb
 *
 * Sets the callback to relay logging messages to the binding
 *
 * @param cb The callback to call, or NULL to stop relaying messages
 * @param min_level The minimum logging level for which to relay messages
 *
 * @return whether the operation succeeded or not
 **/
bool ddwaf_set_log_cb(ddwaf_log_cb cb, DDWAF_LOG_LEVEL min_level);

#ifdef __cplusplus
}
#endif /* __cplusplus */

#endif /* pw_h */
