// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_RTLOADER_H
#define DATADOG_AGENT_RTLOADER_RTLOADER_H

#include "rtloader_types.h"

#include <map>
#include <mutex>
#include <string>
#include <vector>

//! RtLoaderPyObject class.
/*!
  A PyObject C++ class representation for C-API PyObjects.
*/
class RtLoaderPyObject
{
};

//! RtLoader class.
/*!
  RtLoader provides the virtual base class interface for runtime implementations.

  The goal of this class is to provide a simple, comprehensive interface to allow
  implementing runtimes to integrate with the agent and run integration checks.
*/
class RtLoader
{
public:
    //! Constructor.
    RtLoader(cb_memory_tracker_t memtrack_cb);

    //! Destructor.
    virtual ~RtLoader(){};

    // Public API
    //! Pure virtual init member.
    /*!
      This method initializes the underlying runtime
    */
    virtual bool init() = 0;

    //! Pure virtual addPythonPath member.
    /*!
      This method adds a python path to the underlying python runtime.
    */
    virtual bool addPythonPath(const char *path) = 0;

    //! Pure virtual GILEnsure member.
    /*!
      \return A rtloader_gilstate_t GIL state lock reference
      This method ensures the python GIL for the underlying runtime is locked.
    */
    virtual rtloader_gilstate_t GILEnsure() = 0;

    //! Pure virtual GILRelease member.
    /*!
      \param state A rtloader_gilstate_t GIL state lock reference - typically returned by GILEnsure
      \sa GILEnsure()
      This method ensures the python GIL for the underlying runtime is released.
    */
    virtual void GILRelease(rtloader_gilstate_t) = 0;

    //! Pure virtual getClass member.
    /*!
     *
      \brief This member function attemts to find a valid check class within the specified
      specified Python module.
      \param module A C-string representation of the module class we wish to get.
      \param pyModule The python module we wish to load the class from.
      \param pyClass The output python object pointer to the loaded class, if we succeed.
      \return A boolean indicating the success or not of the operation.
    */
    virtual bool getClass(const char *module, RtLoaderPyObject *&pyModule, RtLoaderPyObject *&pyClass) = 0;

    //! Pure virtual getAttrString member.
    /*!
      \param obj The python object we wish to get the string attribute by name from.
      \param attributeName A C-string representation of the string attribute we wish to get by name.
      \param value The output C-string representation to the specified attribute, if we succeed.
      \return A boolean indicating the success or not of the operation.
    */
    virtual bool getAttrString(RtLoaderPyObject *obj, const char *attributeName, char *&value) const = 0;

    //! Pure virtual getCheck member.
    /*!
      \param py_class The python check class we wish to instantiate.
      \param init_config_str A C-string containing the init_config for the check instance.
      \param instance_str A C-string containing the instance config for the check instance.
      \param check_id_str A C-string containing the identifier for the check instance.
      \param check_name A C-string containing the check name.
      \param agent_config_str A C-string containing the full agent configuration.
      \param check The output python object pointer to the instantiated check, if we succeed.
      \return A boolean indicating the success or not of the operation.
    */
    virtual bool getCheck(RtLoaderPyObject *py_class, const char *init_config_str, const char *instance_str,
                          const char *check_id_str, const char *check_name, const char *agent_config_str,
                          RtLoaderPyObject *&check)
        = 0;

    //! Pure virtual runCheck member.
    /*!
      \param check The python object pointer to the check we wish to run.
      \return A C-string with the check result.
    */
    virtual char *runCheck(RtLoaderPyObject *check) = 0;

    //! Pure virtual cancelCheck member.
    /*!
      \param check The python object pointer to the check we wish to cancel.
    */
    virtual void cancelCheck(RtLoaderPyObject *check) = 0;

    //! Pure virtual getCheckWarnings member.
    /*!
      \param check The python object pointer to the check we wish to collect existing warnings for.
      \return An array of C-strings containing all warnings presently set for the check instance.
    */
    virtual char **getCheckWarnings(RtLoaderPyObject *check) = 0;

    //! Pure virtual getCheckDiagnoses member.
    /*!
      \param check The python object pointer to the check we wish to collect diagnoses for.
      \return diagnoses_t (an array diagnoses_t of diagnosis_t containing all diagnoses for the
      check instance.
    */
    virtual diagnoses_t *getCheckDiagnoses(RtLoaderPyObject *check) = 0;

    //! clearError member.
    /*!
      Clears any errors set on the RtLoader instance.
    */
    void clearError();

    //! free member.
    /*!
      \param pointer the memory region on the heap we wish to free.
      Helper member to free heap memory.
    */
    void free(void *);

    //! Pure virtual decref member.
    /*!
      \param The python object pointer we wish to decrement the reference count for.
    */
    virtual void decref(RtLoaderPyObject *) = 0;

    //! Pure virtual incref member.
    /*!
      \param The python object pointer we wish to increment the reference count for.
    */
    virtual void incref(RtLoaderPyObject *) = 0;

    //! Pure virtual setModuleAttrString member.
    /*!
      \param module A C-string representation with the module name we wish to set the attribute for..
      \param attr A C-string representation of the attribute name we wish to add to the module.
      \param attr A C-string representation of the value for the attribute we wish to add.
    */
    virtual void setModuleAttrString(char *module, char *attr, char *value) = 0;

    // Public Const API
    //! Pure virtual getPyInfo member.
    /*!
      \return A py_info_t struct with the details (version and path) of the underlying python runtime.

      Structure returned must be freed with a call to freePyInfo()
    */
    virtual py_info_t *getPyInfo() = 0;

    // Public Const API
    //! Pure virtual freePyInfo member
    /*!
      \return none

      Frees all memory allocated in a previous call to getPyInfo
     */
    virtual void freePyInfo(py_info_t *) = 0;

    //! Pure virtual runSimpleString member.
    /*!
      \param code A C-string representation of python code we wish to run on the underlying python runtime.
      \return A boolean with the status of attempting to run the input code on the underlying python runtime.
    */
    virtual bool runSimpleString(const char *code) const = 0;

    //! Pure virtual getNone member.
    /*!
      \return A RtLoaderPyObject pointer to the python object representing None.
      \sa RtLoaderPyObject
    */
    virtual RtLoaderPyObject *getNone() const = 0;

    //! getError member.
    /*!
      \return The C-string representation of whatever error is currently set in the RtLoader instance.
    */
    const char *getError() const;

    //! hasError member.
    /*!
      \return A boolean indicating if any error has been sot on the RtLoader instance.
    */
    bool hasError() const;

    //! setError member.
    /*!
      \param msg A string with the error message we wish to set on rtloader.

      Only const members should be setting errors on the RtLoader instance.
    */
    void setError(const std::string &msg) const; // let const methods set errors

    //! setError member.
    /*!
      \param msg A C-string representation with the error message we wish to set on rtloader.

      Only const members should be setting errors on the RtLoader instance.
    */
    void setError(const char *msg) const;

    // Python Helpers
    //! getIntegrationList member.
    /*!
      \return A yaml-encoded C-string with the list of every datadog integration wheel installed.
    */
    virtual char *getIntegrationList() = 0;
#define _PY_MEM_MODULE "utils.py_mem"
#define _PY_MEM_SUMMARY_FUNC "get_mem_stats"
    virtual char *getInterpreterMemoryUsage() = 0;

    // aggregator API
    //! setSubmitMetricCb member.
    /*!
      \param A cb_submit_metric_t function pointer to the CGO callback.

      Actual metrics are submitted from go-land, this allows us to set the CGO callback.
    */
    virtual void setSubmitMetricCb(cb_submit_metric_t) = 0;

    //! setSubmitServiceCheckCb member.
    /*!
      \param A cb_submit_service_check_t function pointer to the CGO callback.

      Actual service checks are submitted from go-land, this allows us to set the CGO callback.
    */
    virtual void setSubmitServiceCheckCb(cb_submit_service_check_t) = 0;

    //! setSubmitEventCb member.
    /*!
      \param A cb_submit_event_t function pointer to the CGO callback.

      Actual events are submitted from go-land, this allows us to set the CGO callback.
    */
    virtual void setSubmitEventCb(cb_submit_event_t) = 0;

    //! setSubmitHistogramBucketCb member.
    /*!
      \param A cb_submit_histogram_bucket_t function pointer to the CGO callback.

      Actual histogram buckets are submitted from go-land, this allows us to set the CGO callback.
    */
    virtual void setSubmitHistogramBucketCb(cb_submit_histogram_bucket_t) = 0;

    //! setSubmitEventPlatformEventCb member.
    /*!
      \param A cb_submit_event_platform_event_t function pointer to the CGO callback.

      Actual events are submitted from go-land, this allows us to set the CGO callback.
    */
    virtual void setSubmitEventPlatformEventCb(cb_submit_event_platform_event_t) = 0;

    // datadog_agent API

    //! setGetVersionCb member.
    /*!
      \param A cb_get_version_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will enable us to get the agent version.
    */
    virtual void setGetVersionCb(cb_get_version_t) = 0;

    //! setGetConfigCb member.
    /*!
      \param A cb_get_config_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will enable us to get the agent configuration.
    */
    virtual void setGetConfigCb(cb_get_config_t) = 0;

    //! setHeadersCb member.
    /*!
      \param A cb_headers_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will provide HTTP headers for requests.
    */
    virtual void setHeadersCb(cb_headers_t) = 0;

    //! setGetHostnameCb member.
    /*!
      \param A cb_get_hostname_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will provide the canonical hostname from
      the agent.
    */
    virtual void setGetHostnameCb(cb_get_hostname_t) = 0;

    //! setGetTracemallocEnabledCb member.
    /*!
      \param A cb_tracemalloc_enabled_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will provide the tracemalloc enabled
      configuration setting.
    */
    virtual void setGetTracemallocEnabledCb(cb_tracemalloc_enabled_t) = 0;

    //! setGetClusternameCb member.
    /*!
      \param A cb_get_clustername_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will provide the kubernetes cluster name from
      the agent.
    */
    virtual void setGetClusternameCb(cb_get_clustername_t) = 0;

    //! setLogCb member.
    /*!
      \param A cb_log_t function pointer to the CGO callback.

      This allows us to set the CGO callback that will allow any logging from rtloader or any
      underlying runtimes to be handled by the go logging facilities in the agent, effectively
      providing a single logging subsystem.
    */
    virtual void setLogCb(cb_log_t) = 0;

    //! setCheckMetadataCb member.
    /*!
      \param A cb_set_check_metadata_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow adding metadata for
      specific check instances to the go-land Inventories metadata provider cache.
    */
    virtual void setSetCheckMetadataCb(cb_set_check_metadata_t) = 0;

    //! setExternalTagsCb member.
    /*!
      \param A cb_set_external_tags_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow adding sets of tags for
      specific hostnames to the go-land External Host Tags metadata provider cache.
    */
    virtual void setSetExternalTagsCb(cb_set_external_tags_t) = 0;

    // _util API
    //! setSubprocessOutputCb member.
    /*!
      \param A cb_get_subprocess_output_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow running subprocess
      commands from go-land, where we have some additional helpers for the task.
    */
    virtual void setSubprocessOutputCb(cb_get_subprocess_output_t) = 0;

    // CGO API
    //! setCGOFreeCb member.
    /*!
      \param A cb_cgo_free_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow freeing memory that
      was allocated from CGO, also from CGO. This is a memory safety requirement imposed by
      windows platforms. Other than a slight performance overhead using the callback should
      be equivalent to a regular free().
    */
    virtual void setCGOFreeCb(cb_cgo_free_t) = 0;

    // tagger API
    //! setTagsCb member.
    /*!
      \param A cb_tags_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback to retrieve container tags
      from the agent Tagger component.
    */
    virtual void setTagsCb(cb_tags_t) = 0;

    // kubeutil API
    //! setGetConnectionInfoCb member.
    /*!
      \param A cb_get_connection_info_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback to retrieve the K8s cluster
      connection information from the agent.
    */
    virtual void setGetConnectionInfoCb(cb_get_connection_info_t) = 0;

    // containers API
    //! setIsExcludedCb member.
    /*!
      \param A cb_is_excluded_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback to verify if a certain
      container name or image is excluded from collection.
    */
    virtual void setIsExcludedCb(cb_is_excluded_t) = 0;

    //! setWritePersistentCacheCb member.
    /*!
      \param A cb_write_persistent_cache_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow storing value for
      specific check instances.
    */
    virtual void setWritePersistentCacheCb(cb_write_persistent_cache_t) = 0;

    //! setReadPersistentCacheCb member.
    /*!
      \param A cb_read_persistent_cache_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow retrieving value for
      specific check instances.
    */
    virtual void setReadPersistentCacheCb(cb_read_persistent_cache_t) = 0;

    //! setObfuscateSqlCb member.
    /*!
      \param A cb_obfuscate_sql_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow retrieving value for
      specific check instances.
    */
    virtual void setObfuscateSqlCb(cb_obfuscate_sql_t) = 0;

    //! setObfuscateSqlExecPlanCb member.
    /*!
      \param A cb_obfuscate_sql_exec_plan_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow retrieving value for
      specific check instances.
    */
    virtual void setObfuscateSqlExecPlanCb(cb_obfuscate_sql_exec_plan_t) = 0;

    //! setGetProcessStartTimeCb member.
    /*!
      \param A cb_get_process_start_time_t function pointer to the CGO callback.

      This allows us to set the relevant CGO callback that will allow retrieving value for
      specific check instances.
    */
    virtual void setGetProcessStartTimeCb(cb_get_process_start_time_t) = 0;

    //! initPymemStats member.
    /*!
      Install python allocator hooks.
    */
    virtual void initPymemStats()
    {
    }

    //! getPymemStats member.
    /*!
      \param stats Stats snapshot output.

      Retrieve a snapshot python allocator statistics.
    */
    virtual void getPymemStats(pymem_stats_t &stats)
    {
    }

private:
    mutable std::string _error; /*!< string containing a RtLoader error */
    mutable bool _errorFlag; /*!< boolean indicating whether an error was set on RtLoader */
};

/*! create_t function prototype
  \typedef create_t defines the factory function prototype to create RtLoader instances for
  the underlying python runtimes.
  \param python_home A C-string path to the python home for the target python runtime.
  \param python_exe A C-string path to the python interpreter.
  \return A pointer to the RtLoader instance created by the implementing function.
*/
typedef RtLoader *(create_t)(const char *python_home, const char *python_exe, cb_memory_tracker_t memtrack_cb);

/*! destroy_t function prototype
  \typedef destroy_t defines the destructor function prototype to destroy existing RtLoader instances.
  \param A RtLoader object pointer to the instance that should be destroyed.
*/
typedef void(destroy_t)(RtLoader *);

#ifndef _WIN32
/*! core_trigger_t function pointer
  \brief function pointer to the core triggering routine.
  \param An integer corresponding to the signal number that triggered the dump.
*/
typedef void (*core_trigger_t)(int);
#endif

#endif
