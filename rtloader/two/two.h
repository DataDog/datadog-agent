// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_TWO_H
#define DATADOG_AGENT_RTLOADER_TWO_H

// Some preprocessor sanity for builds (2+3 common sources)
#ifndef DATADOG_AGENT_TWO
#    error Build requires defining DATADOG_AGENT_TWO
#elif defined(DATADOG_AGENT_TWO) && defined(DATADOG_AGENT_THREE)
#    error "DATADOG_AGENT_TWO and DATADOG_AGENT_THREE are mutually exclusive - define only one of the two."
#endif

#include <map>
#include <string>
#include <vector>

#include <Python.h>
#include <rtloader.h>

class Two : public RtLoader
{
public:
    //! Constructor.
    /*!
      \param python_home A C-string with the path to the python home for the
      python interpreter.

      Basic constructor, initializes the _error string to an empty string and
      errorFlag to false and set the supplied PYTHONHOME.
    */
    Two(const char *python_home);

    //! Destructor.
    /*!
      Destroys the Two instance, including relevant python teardown calls.

      We do not call Py_Finalize() since we won't be calling it from the same
      thread where we called Py_Initialize(), this is a product of the go runtime
      switch threads constantly. It's not really an issue here as we destroy this
      class instance just before exiting the agent.
      Calling Py_Finalize from a different thread cause the "threading"
      package to raise an exception: "Exception KeyError: KeyError(<current
      thread id>,) in <module 'threading'".
      Even if Python ignores it, the exception ends up in the log files for
      upstart/syslog/...
      Since we don't call Py_Finalize, we don't free _pythonHome here either.

      More info here:
      https://stackoverflow.com/questions/8774958/keyerror-in-module-threading-after-a-successful-py-test-run/12639040#12639040

    */
    ~Two();

    bool init();
    bool addPythonPath(const char *path);
    rtloader_gilstate_t GILEnsure();
    void GILRelease(rtloader_gilstate_t);

    bool getClass(const char *module, RtLoaderPyObject *&pyModule, RtLoaderPyObject *&pyClass);
    bool getAttrString(RtLoaderPyObject *obj, const char *attributeName, char *&value) const;
    bool getCheck(RtLoaderPyObject *py_class, const char *init_config_str, const char *instance_str,
                  const char *check_id_str, const char *check_name, const char *agent_config_str,
                  RtLoaderPyObject *&check);

    char *runCheck(RtLoaderPyObject *check);
    char **getCheckWarnings(RtLoaderPyObject *check);
    void decref(RtLoaderPyObject *obj);
    void incref(RtLoaderPyObject *obj);
    void setModuleAttrString(char *module, char *attr, char *value);

    // const API
    py_info_t *getPyInfo();
    bool runSimpleString(const char *code) const;
    RtLoaderPyObject *getNone() const
    {
        return reinterpret_cast<RtLoaderPyObject *>(Py_None);
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
    //! initPythonHome member.
    /*!
      \brief This member function sets the Python home for the underlying python2.7 interpreter.
      \param pythonHome A C-string to the target python home for the python runtime.
    */
    void initPythonHome(const char *pythonHome = NULL);

    //! _importFrom member.
    /*!
      \brief This member function imports a Python object by name from the specified
      module.
      \param module A C-string representation of the Python module we wish to import from.
      \param name A C-string representation of the target Python object we wish to import.
      \return A PyObject * pointer to the imported Python object, or NULL in case of error.

      This function returns a new reference to the underlying PyObject. In case of error,
      NULL is returned with clean interpreter error flag.
    */
    PyObject *_importFrom(const char *module, const char *name);

    //! _findSubclassOf member.
    /*!
      \brief This member function attemts to find a subclass of the provided base class in
      the specified Python module.
      \param base A PyObject * pointer to the Python base class we wish to search for.
      \param moduleName A PyObject * pointer to the module we wish to find a derived class
      in.
      \return A PyObject * pointer to the found subclass Python object, or NULL in case of error.

      This function returns a new reference to the underlying PyObject. In case of error,
      NULL is returned with clean interpreter error flag.
    */
    PyObject *_findSubclassOf(PyObject *base, PyObject *moduleName);

    //! _fetchPythonError member.
    /*!
      \brief This member function retrieves the error set on the python interpreter.
      \return A string describing the python error/exception set on the underlying python
      interpreter.
    */
    std::string _fetchPythonError();

    /*! PyPaths type prototype
      \typedef PyPaths defines a vector of strings.
    */
    typedef std::vector<std::string> PyPaths;

    char *_pythonHome; /*!< string with the PYTHONHOME for the underlying interpreter */
    PyObject *_baseClass; /*!< PyObject * pointer to the base Agent check class */
    PyPaths _pythonPaths; /*!< string vector containing paths in the PYTHONPATH */
    PyThreadState *_threadState; /*!< PyThreadState * pointer to the saved Python interpreter thread state */
};

#endif
