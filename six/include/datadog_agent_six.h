// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_H_INCLUDED
#define DATADOG_AGENT_SIX_H_INCLUDED

/*! \file datadog_agent_six.h
    \brief Datadog Agent Six interface header file.

    The prototypes here defined provide a set of functions which either
    present the API to the datadog-agent to call via CGO, or helpers
    relevant to that API. The goal of this interface is to provide a
    simple, basic set of calls to enable any backend to run checks for
    the agent.
*/

#include <six_types.h>

#ifdef __cplusplus
extern "C" {
#endif

struct six_s;
typedef struct six_s six_t;

struct six_pyobject_s;
typedef struct six_pyobject_s six_pyobject_t;


// FACTORIES
/*! \fn six_t *make2(const char *python_home, char **error)
    \brief Factory function to load the python2 backend DLL and create its relevant Six
    instance.
    \param python_home A C-string containing the expected PYTHONhOME for said DLL.
    \param error A C-stringi pointer output parameter to return error messages.
    \return A six_t * pointer to the Six instance.
    \sa six_t
*/
DATADOG_AGENT_SIX_API six_t *make2(const char *pythonhome, char **error);
/*! \fn six_t *make3(const char *python_home, char **error)
    \brief Factory function to load the python3 backend DLL and create its relevant Six
    instance.
    \param python_home A C-string containing the expected PYTHONhOME for said DLL.
    \param error A C-stringi pointer output parameter to return error messages.
    \return A six_t * pointer to the Six instance.
    \sa six_t
*/
DATADOG_AGENT_SIX_API six_t *make3(const char *pythonhome, char **error);


// API
/*! \fn void destroy(six_t *six)
    \brief Destructor function for the provided six backend.
    \param six_t A six_t * pointer to the Six instance we wish to destroy.
    \sa six_t
*/
DATADOG_AGENT_SIX_API void destroy(six_t *);

/*! \fn int init(six_t *six)
    \brief Initializing function for the supplied Six instance.
    \param six_t A six_t * pointer to the Six instance we wish to initialize.
    \return An integer with the success of the operation. Zero for success,
    non-zero for failure.
    \sa six_t

    This function should be called prior to any further interaction with the
    Six.
*/
DATADOG_AGENT_SIX_API int init(six_t *);

/*! \fn int add_python_path(six_t *, const char *path)
    \brief Adds a path to the list of python paths.
    \param six_t A six_t * pointer to the Six instance.
    \param path A C-string containing the path we wish to add to `sys.paths`.
    \return An integer with the success of the operation. Zero for success,
    non-zero for failure.
    \sa six_t
*/
DATADOG_AGENT_SIX_API int add_python_path(six_t *, const char *path);

/*! \fn void clear_error(six_t *)
    \brief Clears any error set in the Six instance.
    \param six_t A six_t * pointer to the Six instance.
    \sa six_t
*/
DATADOG_AGENT_SIX_API void clear_error(six_t *);

/*! \fn six_gilstate_t ensure_gil(six_t *)
    \brief Ensures we have the python GIL lock and returns the GIL lock state.
    \param six_t A six_t * pointer to the Six instance.
    \return A six_gilstate_t type with the GIL state.
    \sa six_gilstate_t, six_t
*/
DATADOG_AGENT_SIX_API six_gilstate_t ensure_gil(six_t *);

/*! \fn void release_gil(six_t *, six_gilstate_t)
    \brief Releases the current python GIL lock being held.
    \param six_t A six_t * pointer to the Six instance.
    \param six_gilstate_t A six_gilstate_t type with the current GIL state.
    \sa six_gilstate_t, six_t
*/
DATADOG_AGENT_SIX_API void release_gil(six_t *, six_gilstate_t);

/*! \fn int get_class(six_t *six, const char *name, six_pyobject_t **py_module,
                                    six_pyobject_t **py_class)
    \brief Attempts to get a python class by name from a specified python module.
    \param six_t A six_t * pointer to the Six instance.
    \param name A constant C-string with the name of the class to get.
    \param py_module A six_pyobject_t ** pointer to the module we wish to get the
    class from.
    \param py_class A six_pyobject_t ** pointer output parameter with a reference to
    the class, or NULL if none-found.
    \sa six_pyobject_t, six_t
*/
DATADOG_AGENT_SIX_API int get_class(six_t *six, const char *name, six_pyobject_t **py_module,
                                    six_pyobject_t **py_class);

/*! \fn int get_attr_string(six_t *six, six_pyobject_t *py_class, const char *attr_name, char **value)
    \brief Attempts to get a string attribute from the supplied python class, by name.
    \param six_t A six_t * pointer to the Six instance.
    \param py_class A six_pyobject_t ** pointer to the class we wish to get the
    attribute from.
    \param attr_name A constant C-string with the name of the attribute to get.
    \param value A char ** pointer C-string output parameter with the attribute value.
    \return An integer with the success of the operation. Zero for success, non-zero for failure.
    \sa six_pyobject_t, six_t
*/
DATADOG_AGENT_SIX_API int get_attr_string(six_t *six, six_pyobject_t *py_class, const char *attr_name, char **value);

/*! \fn int get_check(six_t *six, six_pyobject_t *py_class, const char *init_config, const char *instance,
                                    const char *check_id, const char *check_name, six_pyobject_t **check)
    \brief Attempts to instantiate a datadog python check with the supplied configuration
    parameters.
    \param six_t A six_t * pointer to the Six instance.
    \param py_class A six_pyobject_t * pointer to the python check class we wish to instantiate.
    \param init_config A constant C-string with the init config for the check instance.
    \param instance A constant C-string with the instance-specific config for the check instance.
    \param check_id A constant C-string unique identifier for the check instance.
    \param check_name A constant C-string with the check name.
    \param check A six_pyobject_t ** pointer to the check instantiated if successful or NULL otherwise.
    \return An integer with the success of the operation. Zero for success, non-zero for failure.
    \sa six_pyobject_t, six_t
*/
DATADOG_AGENT_SIX_API int get_check(six_t *six, six_pyobject_t *py_class, const char *init_config, const char *instance,
                                    const char *check_id, const char *check_name, six_pyobject_t **check);

/*! \fn int get_check_deprecated(six_t *six, six_pyobject_t *py_class, const char *init_config,
                                               const char *instance, const char *check_id, const char *check_name,
                                               const char *agent_config, six_pyobject_t **check)
    \brief Attempts to instantiate a datadog python check with the supplied configuration
    parameters.
    \param six_t A six_t * pointer to the Six instance.
    \param py_class A six_pyobject_t * pointer to the python check class we wish to instantiate.
    \param init_config A constant C-string with the init config for the check instance.
    \param instance A constant C-string with the instance-specific config for the check instance.
    \param check_id A constant C-string unique identifier for the check instance.
    \param check_name A constant C-string with the check name.
    \param agent_config A constant C-string with the agent_config.
    \param check A six_pyobject_t ** pointer to the check instantiated if successful or NULL otherwise.
    \return An integer with the success of the operation. Zero for success, non-zero for failure.
    \sa six_pyobject_t, six_t, get_check

    This function is deprecated in favor of `get_check()`.
*/
DATADOG_AGENT_SIX_API int get_check_deprecated(six_t *six, six_pyobject_t *py_class, const char *init_config,
                                               const char *instance, const char *check_id, const char *check_name,
                                               const char *agent_config, six_pyobject_t **check);

/*! \fn const char *run_check(six_t *, six_pyobject_t *check)
    \brief Runs a check instance.
    \param six_t A six_t * pointer to the Six instance.
    \param check A six_pyobject_t * pointer to the check instance we wish to run.
    \return A const C-string with the check summary.
    \sa six_pyobject_t, six_t

    This function is deprecated in favor of `get_check()`.
*/
DATADOG_AGENT_SIX_API const char *run_check(six_t *, six_pyobject_t *check);

/*! \fn char **get_checks_warnings(six_t *, six_pyobject_t *check)
    \brief Get all warnings, if any, for a check instance.
    \param six_t A six_t * pointer to the Six instance.
    \param check A six_pyobject_t * pointer to the check instance we wish to collect the
    warnings for.
    \return An array of C-strings with found warnings for the instance, or NULL if none or
    an error occurred.
    \sa six_pyobject_t, six_t

    This function is deprecated in favor of `get_check()`.
*/
DATADOG_AGENT_SIX_API char **get_checks_warnings(six_t *, six_pyobject_t *check);

/*! \fn void six_free(six_t *, void *ptr)
    \brief Routine to free heap memory in Six.
    \param six_t A six_t * pointer to the Six instance.
    \param ptr A void * pointer to the region of memory we wish to free.
    \sa six_t
*/
DATADOG_AGENT_SIX_API void six_free(six_t *, void *ptr);

/*! \fn void six_decref(six_t *, six_pyobject_t *)
    \brief Routine to decrease the python reference count for the supplied python
    object.
    \param six_t A six_t * pointer to the Six instance.
    \param pyobject A six_pyobject_t * pointer to the python object we wish to decrease the
    reference for.
    \sa six_pyobject_t, six_t
*/
DATADOG_AGENT_SIX_API void six_decref(six_t *, six_pyobject_t *);

/*! \fn void six_incref(six_t *, six_pyobject_t *)
    \brief Routine to increase the python reference count for the supplied python
    object.
    \param six_t A six_t * pointer to the Six instance.
    \param pyobject A six_pyobject_t * pointer to the python object we wish to increase the
    reference for.
    \sa six_pyobject_t, six_t
*/
DATADOG_AGENT_SIX_API void six_incref(six_t *, six_pyobject_t *);

/*! \fn void set_module_attr_string(six_t *, char *, char *, char *)
    \brief Routine to set a string attribute on a given module.
    \param six_t A six_t * pointer to the Six instance.
    \param char * A C-string pointer containing the python module name we wish to add the
    attribute to.
    \param char * A C-string pointer with the attribute name we wish to add.
    \param char * A C-string pointer with the attribute string value.
    \sa six_t
*/
DATADOG_AGENT_SIX_API void set_module_attr_string(six_t *, char *, char *, char *);


// CONST API
/*! \fn six_pyobject_t *get_none(const six_t *)
    \brief Routine to set a string attribute on a given module.
    \param six_t A six_t * pointer to the Six instance.
    \return A six_pyobject_t * pointer to the python `None` object.
    \sa six_pyobject_t, six_t

    Returns a new reference, and thus increases the reference count to `None`.
*/
DATADOG_AGENT_SIX_API six_pyobject_t *get_none(const six_t *);

/*! \fn py_info_t *get_py_info(six_t *)
    \brief Routine to collect python runtime information details from the Six instance.
    \param six_t A six_t * pointer to the Six instance.
    \return A py_info_t * pointer with the relevant python information or NULL in case of
    error.
    \sa py_info_t, six_t

    Allocates memory for the returned `py_info_t` structure and should be freed accordingly.
*/
DATADOG_AGENT_SIX_API py_info_t *get_py_info(six_t *);

/*! \fn int run_simple_string(const six_t *, const char *code)
    \brief Routine to execute a simple piece of python code on the Six python runtime
    implementation.
    \param six_t A const six_t * pointer to the Six instance.
    \param code A const C-string pointer to the simple python code to run on the interpreter.
    \return An integer reflecting whether the code executed successfully on Six. Zero for false,
    non-zero for true.
    \sa six_t

    Allocates memory for the returned `py_info_t` structure and should be freed accordingly.
*/
DATADOG_AGENT_SIX_API int run_simple_string(const six_t *, const char *code);

/*! \fn int has_error(six_t *)
    \brief Routine indicating whether any error is set on the provided Six instance.
    \param six_t A six_t * pointer to the Six instance.
    \return An integer reflecting if an error is set on Six. Zero for false, non-zero for
    true.
    \sa six_t

    No memory is allocated by this function, the returned pointer points to the internal
    array employed by the underlying Six implementation.
*/
DATADOG_AGENT_SIX_API int has_error(const six_t *);

/*! \fn const char *get_error(six_t *)
    \brief Routine to grab any set error on the provided Six instance.
    \param six_t A six_t * pointer to the Six instance.
    \return A constant C-string pointer to the error C-string representation.
    \sa six_t

    No memory is allocated by this function, the returned pointer points to the internal
    array employed by the underlying Six implementation.
*/
DATADOG_AGENT_SIX_API const char *get_error(const six_t *);
#ifndef _WIN32

/*! \fn int handle_crashes(const six_t *, const int)
    \brief Routine to install a crash handler in C-land to better debug crashes on Six.
    \param six_t A six_t * pointer to the Six instance.
    \param int A const integer boolean flag indicating whether dumps should be created
    on crashes or not.
    \return An integer reflecting if the handler was correctly installed on Six. Zero for
    false, non-zero for true.
    \sa six_t

    If core dumps are enabled, you will NOT get the go-routine dump in the event of a crash.
    Core dumps generated from go-land are not as useful as C-stack has unwound, and so we
    get no real visibility into how six may have crashed. On the counterpart, when generating
    the core dumps from C-land, we terminate early, and miss the Go panic handler that would
    provide the go-routine dump. If you need both, just crash twice trying both options :)

    Currently only SEGFAULT is handled.
*/
DATADOG_AGENT_SIX_API int handle_crashes(const six_t *, const int);
#endif

// PYTHON HELPERS
/*! \fn char *get_integration_list(six_t *)
    \brief Routine to build a list of every datadog wheel installed.
    \param six_t A six_t * pointer to the Six instance.
    \return A C-string with the list of every datadog integration wheel installed.
    \sa six_t

    The returned list must be freed by the caller.
*/
DATADOG_AGENT_SIX_API char *get_integration_list(six_t *);


// AGGREGATOR API
/*! \fn void set_submit_metric_cb(six_t *, cb_submit_metric_t)
    \brief Sets the submit metric callback to be used by six for metric submission.
    \param six_t A six_t * pointer to the Six instance.
    \param cb A function pointer with cb_submit_metric_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_submit_metric_cb(six_t *, cb_submit_metric_t);

/*! \fn void set_submit_service_check_cb(six_t *, cb_submit_service_check_t)
    \brief Sets the submit service_check callback to be used by six for service_check
    submission.
    \param cb A function pointer with cb_submit_service_check_t prototype to the
    callback function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_submit_service_check_cb(six_t *, cb_submit_service_check_t);

/*! \fn void set_submit_event_cb(six_t *, cb_submit_event_t)
    \brief Sets the submit event callback to be used by six for event submission.
    \param cb A function pointer with cb_submit_event_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_submit_event_cb(six_t *, cb_submit_event_t);


// DATADOG_AGENT API
/*! \fn void set_get_version_cb(six_t *, cb_get_version_t)
    \brief Sets a callback to be used by six to collect the agent version.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_version_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_version_cb(six_t *, cb_get_version_t);

/*! \fn void set_get_config_cb(csix_t *, b_get_config_t)
    \brief Sets a callback to be used by six to collect the agent configuration.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_config_t prototype to the
    callback function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_config_cb(six_t *, cb_get_config_t);

/*! \fn void set_headers_cb(six_t *, cb_headers_t)
    \brief Sets a callback to be used by six to collect the typical HTTP headers for
    agent requests.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_headers_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_headers_cb(six_t *, cb_headers_t);

/*! \fn void set_get_hostname_cb(six_t *, cb_get_hostname_t)
    \brief Sets a callback to be used by six to collect the canonical hostname from the
    agent.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_hostname_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_hostname_cb(six_t *, cb_get_hostname_t);

/*! \fn void set_get_clustername_cb(six_t *, cb_get_clustername_t)
    \brief Sets a callback to be used by six to collect the K8s clustername from the
    agent.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_clustername_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_clustername_cb(six_t *, cb_get_clustername_t);

/*! \fn void set_log_cb(six_t *, cb_log_t)
    \brief Sets a callback to be used by six to allow using the agent's go-native
    logging facilities to log messages.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_log_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_log_cb(six_t *, cb_log_t);

/*! \fn void set_set_external_tags_cb(six_t *, cb_set_external_tags_t)
    \brief Sets a callback to be used by six to allow setting external tags for a given
    hostname.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_set_external_tags_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_set_external_tags_cb(six_t *, cb_set_external_tags_t);

// _UTIL API
/*! \fn void set_get_subprocess_output_cb(six_t *six, cb_get_subprocess_output_t)
    \brief Sets a callback to be used by six to run subprocess commands and collect their
    output.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_subprocess_output_t prototype to the callback
    function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_subprocess_output_cb(six_t *six, cb_get_subprocess_output_t cb);

// CGO API
/*! \fn void set_cgo_free_cb(six_t *six, cb_cgo_free_t cb)
    \brief Sets a callback to be used by six to free memory allocated by the
    six's caller and passed into six.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer to the callback function.

    On Windows we cannot free a memory block from another DLL. This is why we
    need to call back to the allocating DLL if it wishes to release allocated memory.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_cgo_free_cb(six_t *, cb_cgo_free_t);

// TAGGER API
/*! \fn void set_tags_cb(six_t *, cb_tags_t)
    \brief Sets a callback to be used by six for setting the relevant tags.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with the cb_tags_t prototype to the
    callback function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
    The callback in turn will call the pertinent internal go-land tagger logic.
    The callback logic will allocate a C(go) pointer array, and the C strings for the
    tagger generate tags. This memory should be freed with the cgo_free helper
    available when done.
*/
DATADOG_AGENT_SIX_API void set_tags_cb(six_t *, cb_tags_t);

// KUBEUTIL API
/*! \fn void set_get_connection_info_cb(six_t *, cb_get_connection_info_t)
    \brief Sets a callback to be used by six for kubernetes connection information
    retrieval.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_get_connection_info_t prototype to the
    callback function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_get_connection_info_cb(six_t *, cb_get_connection_info_t);

// CONTAINERS
/*! \fn void set_is_excluded_cb(six_t *, cb_is_excluded_t)
    \brief Sets a callback to be used by six to determine if a container is excluded
    from metric collection.
    \param six_t A six_t * pointer to the Six instance.
    \param object A function pointer with cb_is_excluded_t function prototype to the
    callback function.

    The callback is expected to be provided by the six caller - in go-context: CGO.
*/
DATADOG_AGENT_SIX_API void set_is_excluded_cb(six_t *, cb_is_excluded_t);

#ifdef __cplusplus
}
#endif
#endif
