// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_H
#define DATADOG_AGENT_SIX_THREE_H

// Some preprocessor sanity for builds (2+3 common sources)
#ifndef DATADOG_AGENT_THREE
#    error Build requires defining DATADOG_AGENT_THREE
#elif defined(DATADOG_AGENT_TWO) && defined(DATADOG_AGENT_THREE)
#    error "DATADOG_AGENT_TWO and DATADOG_AGENT_THREE are mutually exclusive - define only one of the two."
#endif

#include <map>
#include <mutex>
#include <string>
#include <vector>

#include <Python.h>
#include <six.h>

class Three : public Six
{
public:
    Three(const char *python_home);
    ~Three();

    bool init();
    bool addPythonPath(const char *path);
    six_gilstate_t GILEnsure();
    void GILRelease(six_gilstate_t);

    bool getClass(const char *module, SixPyObject *&pyModule, SixPyObject *&pyClass);
    bool getAttrString(SixPyObject *obj, const char *attributeName, char *&value) const;
    bool getCheck(SixPyObject *py_class, const char *init_config_str, const char *instance_str,
                  const char *check_id_str, const char *check_name, const char *agent_config_str, SixPyObject *&check);

    const char *runCheck(SixPyObject *check);
    char **getCheckWarnings(SixPyObject *check);
    void decref(SixPyObject *obj);
    void incref(SixPyObject *obj);
    void set_module_attr_string(char *module, char *attr, char *value);

    // const API
    bool isInitialized() const;
    py_info_t *getPyInfo();
    bool runSimpleString(const char *path) const;
    SixPyObject *getNone() const
    {
        return reinterpret_cast<SixPyObject *>(Py_None);
    }

    // Python Helpers
    char *getIntegrationList();

    // aggregator API
    void setSubmitMetricCb(cb_submit_metric_t);
    void setSubmitServiceCheckCb(cb_submit_service_check_t);
    void setSubmitEventCb(cb_submit_event_t);

    // datadog_agent API
    void setGetVersionCb(cb_get_version_t);
    void setGetConfigCb(cb_get_config_t);
    void setHeadersCb(cb_headers_t);
    void setGetHostnameCb(cb_get_hostname_t);
    void setGetClusternameCb(cb_get_clustername_t);
    void setLogCb(cb_log_t);
    void setSetExternalTagsCb(cb_set_external_tags_t);

    // _util API
    virtual void setSubprocessOutputCb(cb_get_subprocess_output_t);

    // CGO API
    void setCGOFreeCb(cb_cgo_free_t);

    // tagger
    void setTagsCb(cb_tags_t);

    // kubeutil
    void setGetConnectionInfoCb(cb_get_connection_info_t);

    // containers
    void setIsExcludedCb(cb_is_excluded_t);

private:
    void initPythonHome(const char *pythonHome = NULL);
    PyObject *_importFrom(const char *module, const char *name);
    PyObject *_findSubclassOf(PyObject *base, PyObject *module);
    std::string _fetchPythonError() const;

    typedef std::vector<std::string> PyPaths;

    wchar_t *_pythonHome;
    PyObject *_baseClass;
    PyPaths _pythonPaths;
    PyThreadState *_threadState;
};

#endif
