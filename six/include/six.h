// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_SIX_H
#define DATADOG_AGENT_SIX_SIX_H

#include "six_types.h"
#include <map>
#include <mutex>
#include <string>
#include <vector>

// Opaque type to wrap PyObject
class SixPyObject
{
};

class Six
{
public:
    Six()
        : _error()
        , _errorFlag(false){};
    virtual ~Six(){};

    // Public API
    virtual bool init() = 0;
    virtual bool addPythonPath(const char *path) = 0;
    virtual six_gilstate_t GILEnsure() = 0;
    virtual void GILRelease(six_gilstate_t) = 0;

    virtual bool getClass(const char *module, SixPyObject *&pyModule, SixPyObject *&pyClass) = 0;
    virtual bool getAttrString(SixPyObject *obj, const char *attributeName, char *&value) const = 0;
    virtual bool getCheck(SixPyObject *py_class, const char *init_config_str, const char *instance_str,
                          const char *check_id_str, const char *check_name, const char *agent_config_str,
                          SixPyObject *&check)
        = 0;

    virtual const char *runCheck(SixPyObject *check) = 0;
    virtual char **getCheckWarnings(SixPyObject *check) = 0;
    void clearError();
    void free(void *);
    virtual void decref(SixPyObject *) = 0;
    virtual void incref(SixPyObject *) = 0;
    virtual void set_module_attr_string(char *module, char *attr, char *value) = 0;

    // Public Const API
    virtual bool isInitialized() const = 0;
    virtual py_info_t *getPyInfo() = 0;
    virtual bool runSimpleString(const char *code) const = 0;
    virtual SixPyObject *getNone() const = 0;
    const char *getError() const;
    bool hasError() const;
    void setError(const std::string &msg) const; // let const methods set errors
    void setError(const char *msg) const;
#ifndef _WIN32
    void handleCrashes(const bool coredump) const;
#endif

    // Python Helpers
    virtual char *getIntegrationList() = 0;

    // aggregator API
    virtual void setSubmitMetricCb(cb_submit_metric_t) = 0;
    virtual void setSubmitServiceCheckCb(cb_submit_service_check_t) = 0;
    virtual void setSubmitEventCb(cb_submit_event_t) = 0;

    // datadog_agent API
    virtual void setGetVersionCb(cb_get_version_t) = 0;
    virtual void setGetConfigCb(cb_get_config_t) = 0;
    virtual void setHeadersCb(cb_headers_t) = 0;
    virtual void setGetHostnameCb(cb_get_hostname_t) = 0;
    virtual void setGetClusternameCb(cb_get_clustername_t) = 0;
    virtual void setLogCb(cb_log_t) = 0;
    virtual void setSetExternalTagsCb(cb_set_external_tags_t) = 0;

    // _util API
    virtual void setSubprocessOutputCb(cb_get_subprocess_output_t) = 0;

    // CGO API
    virtual void setCGOFreeCb(cb_cgo_free_t) = 0;

    // tagger API
    virtual void setTagsCb(cb_tags_t) = 0;

    // kubeutil API
    virtual void setGetConnectionInfoCb(cb_get_connection_info_t) = 0;

    // containers API
    virtual void setIsExcludedCb(cb_is_excluded_t) = 0;

private:
    mutable std::string _error;
    mutable bool _errorFlag;
};

typedef Six *create_t(const char *python_home);
typedef void destroy_t(Six *);
typedef void (*core_trigger_t)(int);

#endif
