// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_SIX_THREE_H
#define DATADOG_AGENT_SIX_THREE_H
#include <map>
#include <mutex>
#include <string>
#include <vector>

#include <Python.h>
#include <six.h>

#ifndef DATADOG_AGENT_SIX_API
#    ifdef DATADOG_AGENT_SIX_TEST
#        define DATADOG_AGENT_SIX_API
#    elif _WIN32
#        define DATADOG_AGENT_SIX_API __declspec(dllexport)
#    else
#        if __GNUC__ >= 4
#            define DATADOG_AGENT_SIX_API __attribute__((visibility("default")))
#        else
#            define DATADOG_AGENT_SIX_API
#        endif
#    endif
#endif

class Three : public Six {
public:
    Three()
        : _pythonHome(NULL)
        , _baseClass(NULL)
        , _pythonPaths(){};
    ~Three();

    bool init(const char *pythonHome);
    bool addPythonPath(const char *path);
    six_gilstate_t GILEnsure();
    void GILRelease(six_gilstate_t);
    bool getCheck(const char *module, const char *init_config, const char *instances, SixPyObject *&check,
                  char *&version);
    const char *runCheck(SixPyObject *check);
    void decref(SixPyObject *);

    // const API
    bool isInitialized() const;
    const char *getPyVersion() const;
    bool runSimpleString(const char *path) const;
    SixPyObject *getNone() const {
        return reinterpret_cast<SixPyObject *>(Py_None);
    }

    // aggregator API
    void setSubmitMetricCb(cb_submit_metric_t);
    void setSubmitServiceCheckCb(cb_submit_service_check_t);
    void setSubmitEventCb(cb_submit_event_t);

    // datadog_agent
    void setGetVersionCb(cb_get_version_t);
    void setGetConfigCb(cb_get_config_t);
    void setHeadersCb(cb_headers_t);

private:
    PyObject *_importFrom(const char *module, const char *name);
    PyObject *_findSubclassOf(PyObject *base, PyObject *module);
    std::string _fetchPythonError() const;
    char *_getCheckVersion(PyObject *module) const;

    typedef std::vector<std::string> PyPaths;

    wchar_t *_pythonHome;
    PyObject *_baseClass;
    PyPaths _pythonPaths;
};

#endif
