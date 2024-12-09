// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "datadog_agent.h"
#include "cgo_free.h"
#include "rtloader_mem.h"
#include "stringutils.h"

#include <log.h>

// these must be set by the Agent
static cb_get_clustername_t cb_get_clustername = NULL;
static cb_get_config_t cb_get_config = NULL;
static cb_get_hostname_t cb_get_hostname = NULL;
static cb_get_host_tags_t cb_get_host_tags = NULL;
static cb_tracemalloc_enabled_t cb_tracemalloc_enabled = NULL;
static cb_get_version_t cb_get_version = NULL;
static cb_headers_t cb_headers = NULL;
static cb_send_log_t cb_send_log = NULL;
static cb_set_check_metadata_t cb_set_check_metadata = NULL;
static cb_set_external_tags_t cb_set_external_tags = NULL;
static cb_write_persistent_cache_t cb_write_persistent_cache = NULL;
static cb_read_persistent_cache_t cb_read_persistent_cache = NULL;
static cb_obfuscate_sql_t cb_obfuscate_sql = NULL;
static cb_obfuscate_sql_exec_plan_t cb_obfuscate_sql_exec_plan = NULL;
static cb_get_process_start_time_t cb_get_process_start_time = NULL;
static cb_obfuscate_mongodb_string_t cb_obfuscate_mongodb_string = NULL;
static cb_emit_agent_telemetry_t cb_emit_agent_telemetry = NULL;

// forward declarations
static PyObject *get_clustername(PyObject *self, PyObject *args);
static PyObject *get_config(PyObject *self, PyObject *args);
static PyObject *get_hostname(PyObject *self, PyObject *args);
static PyObject *get_host_tags(PyObject *self, PyObject *args);
static PyObject *tracemalloc_enabled(PyObject *self, PyObject *args);
static PyObject *get_version(PyObject *self, PyObject *args);
static PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *log_message(PyObject *self, PyObject *args);
static PyObject *send_log(PyObject *self, PyObject *args);
static PyObject *set_check_metadata(PyObject *self, PyObject *args);
static PyObject *set_external_tags(PyObject *self, PyObject *args);
static PyObject *write_persistent_cache(PyObject *self, PyObject *args);
static PyObject *read_persistent_cache(PyObject *self, PyObject *args);
static PyObject *obfuscate_sql(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *obfuscate_sql_exec_plan(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *get_process_start_time(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *obfuscate_mongodb_string(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *emit_agent_telemetry(PyObject *self, PyObject *args, PyObject *kwargs);

static PyMethodDef methods[] = {
    { "get_clustername", get_clustername, METH_NOARGS, "Get the cluster name." },
    { "get_config", get_config, METH_VARARGS, "Get an Agent config item." },
    { "get_hostname", get_hostname, METH_NOARGS, "Get the hostname." },
    { "get_host_tags", get_host_tags, METH_NOARGS, "Get the host tags." },
    { "tracemalloc_enabled", tracemalloc_enabled, METH_VARARGS, "Gets if tracemalloc is enabled." },
    { "get_version", get_version, METH_NOARGS, "Get Agent version." },
    { "headers", (PyCFunction)headers, METH_VARARGS | METH_KEYWORDS, "Get standard set of HTTP headers." },
    { "log", log_message, METH_VARARGS, "Log a message through the agent logger." },
    { "send_log", send_log, METH_VARARGS, "Submit a log for Checks." },
    { "set_check_metadata", set_check_metadata, METH_VARARGS, "Send metadata for Checks." },
    { "set_external_tags", set_external_tags, METH_VARARGS, "Send external host tags." },
    { "write_persistent_cache", write_persistent_cache, METH_VARARGS, "Store a value for a given key." },
    { "read_persistent_cache", read_persistent_cache, METH_VARARGS, "Retrieve the value associated with a key." },
    { "obfuscate_sql", (PyCFunction)obfuscate_sql, METH_VARARGS|METH_KEYWORDS, "Obfuscate & normalize a SQL string." },
    { "obfuscate_sql_exec_plan", (PyCFunction)obfuscate_sql_exec_plan, METH_VARARGS|METH_KEYWORDS, "Obfuscate & normalize a SQL Execution Plan." },
    { "get_process_start_time", (PyCFunction)get_process_start_time, METH_NOARGS, "Get agent process startup time, in seconds since the epoch." },
    { "obfuscate_mongodb_string", (PyCFunction)obfuscate_mongodb_string, METH_VARARGS|METH_KEYWORDS, "Obfuscate & normalize a MongoDB command string." },
    { "emit_agent_telemetry", (PyCFunction)emit_agent_telemetry, METH_VARARGS|METH_KEYWORDS, "Emit agent telemetry." },
    { NULL, NULL } // guards
};

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, DATADOG_AGENT_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void)
{
    return PyModule_Create(&module_def);
}

void _set_get_version_cb(cb_get_version_t cb)
{
    cb_get_version = cb;
}

void _set_get_config_cb(cb_get_config_t cb)
{
    cb_get_config = cb;
}

void _set_headers_cb(cb_headers_t cb)
{
    cb_headers = cb;
}

void _set_get_hostname_cb(cb_get_hostname_t cb)
{
    cb_get_hostname = cb;
}

void _set_get_host_tags_cb(cb_get_host_tags_t cb)
{
    cb_get_host_tags = cb;
}

void _set_get_clustername_cb(cb_get_clustername_t cb)
{
    cb_get_clustername = cb;
}

void _set_send_log_cb(cb_send_log_t cb)
{
    cb_send_log = cb;
}

void _set_set_check_metadata_cb(cb_set_check_metadata_t cb)
{
    cb_set_check_metadata = cb;
}

void _set_write_persistent_cache_cb(cb_write_persistent_cache_t cb)
{
    cb_write_persistent_cache = cb;
}

void _set_read_persistent_cache_cb(cb_read_persistent_cache_t cb)
{
    cb_read_persistent_cache = cb;
}

void _set_set_external_tags_cb(cb_set_external_tags_t cb)
{
    cb_set_external_tags = cb;
}

void _set_tracemalloc_enabled_cb(cb_tracemalloc_enabled_t cb)
{
    cb_tracemalloc_enabled = cb;
}

void _set_obfuscate_sql_cb(cb_obfuscate_sql_t cb)
{
    cb_obfuscate_sql = cb;
}

void _set_obfuscate_sql_exec_plan_cb(cb_obfuscate_sql_exec_plan_t cb)
{
    cb_obfuscate_sql_exec_plan = cb;
}

void _set_get_process_start_time_cb(cb_get_process_start_time_t cb) {
    cb_get_process_start_time = cb;
}

void _set_obfuscate_mongodb_string_cb(cb_obfuscate_mongodb_string_t cb) {
    cb_obfuscate_mongodb_string = cb;

}

void _set_emit_agent_telemetry_cb(cb_emit_agent_telemetry_t cb) {
    cb_emit_agent_telemetry = cb;
}


/*! \fn PyObject *get_version(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.get_version` method, collecting
    the agent version from the agent.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the agent version. Or `None`
    if the callback is unavailable.

    This function is callable as the `datadog_agent.get_version` python method, it uses
    the `cb_get_version()` callback to retrieve the value from the agent with CGO.
*/
PyObject *get_version(PyObject *self, PyObject *args)
{
    if (cb_get_version == NULL) {
        Py_RETURN_NONE;
    }

    char *v = NULL;
    cb_get_version(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        // v is allocated from CGO and thus requires being freed with the
        // cgo_free callback for windows safety.
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/*! \fn PyObject *get_config(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.get_config` method, allowing
    to collect elements in the agent configuration, from the agent.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing a python string.
    \return a PyObject * pointer to a safe unmarshaled python object. Or `None`
    if the callback is unavailable.

    This function is callable as the `datadog_agent.get_config` python method. It
    uses the`cb_get_config()` callback to retrieve the element in the agent configuration
    associated with the key passed in with the args argument. The value returned
    will depend on the element type found for the key, and is a python object
    unmarshaled by the `yaml.safe_load` function when calling `from_yaml()` with
    the payload returned by callback. If no callback is set, `None` will be returned.

    Before RtLoader the Agent used reflection to inspect the contents of a configuration
    value and the CPython API to perform conversion to a Python equivalent. Such
    a conversion wouldn't be possible in a Python-agnostic way so we use YAML to
    pass the data from Go to Python. The configuration value is loaded in the Agent,
    marshalled into YAML and passed as a `char*` to RtLoader, where the string is
    decoded back to Python and passed to the caller. YAML usage is transparent to
    the caller, who would receive a Python object as returned from `yaml.safe_load`.
    YAML is used instead of JSON since the `json.load` return unicode for
    string, for python2, which would be a breaking change from the previous
    version of the agent.
*/
PyObject *get_config(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_config == NULL) {
        Py_RETURN_NONE;
    }

    char *key = NULL;
    // PyArg_ParseTuple returns a pointer to the existing string in &key
    // No need to free the result.
    if (!PyArg_ParseTuple(args, "s", &key)) {
        return NULL;
    }

    char *data = NULL;
    cb_get_config(key, &data);

    // new ref
    PyObject *value = from_yaml(data);
    cgo_free(data);
    if (value == NULL) {
        // clear error set by `from_yaml`
        PyErr_Clear();
        Py_RETURN_NONE;
    }
    return value;
}

/*! \fn PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function provides a standard set of HTTP headers the caller might want to
    use for HTTP requests.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to the `agentConfig`, but not expected to be used.
    \param kwargs A PyObject* pointer to a dictonary. If the `http_host` key is present
    it will be added to the headers.
    \return a PyObject * pointer to a python dictionary with the expected headers.

    This function is callable as the `datadog_agent.headers` python method. The method is
    duplicated and also callable from `util.headers`. `datadog_agent.headers()` isn't used
    by any official integration provided by Datdog but custom checks might still rely on it.
    Currently the contents of the returned string are the same but defined in two
    different places:

     1. github.com/DataDog/integrations-core/blob/master/datadog_checks_base/datadog_checks/base/utils/headers.py
     2. github.com/DataDog/datadog-agent/blob/main/pkg/util/common.go
*/
PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set but be resilient for the Python caller
    if (cb_headers == NULL) {
        Py_RETURN_NONE;
    }

    char *data = NULL;
    cb_headers(&data);

    // new ref
    PyObject *headers_dict = from_yaml(data);
    cgo_free(data);
    if (headers_dict == NULL || !PyDict_Check(headers_dict)) {
        // clear error set by `from_yaml`
        PyErr_Clear();
        // if headers_dict is not a dict we don't need to hold a ref to it
        Py_XDECREF(headers_dict);
        Py_RETURN_NONE;
    }

    // `args` contains `agentConfig` but we don't need it
    // `kwargs` might contain the `http_host` key, let's grab it
    if (kwargs != NULL) {
        char key[] = "http_host";
        // Returns a borrowed reference; no exception set if not present
        PyObject *pyHTTPHost = PyDict_GetItemString(kwargs, key);
        if (pyHTTPHost != NULL) {
            PyDict_SetItemString(headers_dict, "Host", pyHTTPHost);
        }
    }

    return headers_dict;
}

// provide a non-static entry point for the `headers` method; headers is duplicated
// in the `util` module; allow it to be called directly

PyObject *_public_headers(PyObject *self, PyObject *args, PyObject *kwargs)
{
    return headers(self, args, kwargs);
}

/*! \fn PyObject *get_hostname(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.get_hostname` method, collecting
    the canonical hostname from the agent.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the canonical hostname. Or
    `None` if the callback is unavailable.

    This function is callable as the `datadog_agent.get_hostname` python method, it uses
    the `cb_get_hostname()` callback to retrieve the value from the agent with CGO. If
    the callback has not been set `None` will be returned.
*/
PyObject *get_hostname(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_hostname == NULL) {
        Py_RETURN_NONE;
    }

    char *v = NULL;
    cb_get_hostname(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/*! \fn PyObject *get_host_tags(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.get_host_tags` method, collecting
    the host tags from the agent.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the host tags. Or
    `None` if the callback is unavailable.

    This function is callable as the `datadog_agent.get_host_tags` python method, it uses
    the `cb_get_host_tags()` callback to retrieve the value from the agent with CGO. If
    the callback has not been set `None` will be returned.
*/
PyObject *get_host_tags(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_host_tags == NULL) {
        Py_RETURN_NONE;
    }

    char *v = NULL;
    cb_get_host_tags(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/*! \fn PyObject *get_clustername(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.get_clustername` method, collecting
    the K8s clustername from the agent.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the canonical clustername. Or
    `None` if the callback is unavailable.

    This function is callable as the `datadog_agent.get_clustername` python method, it uses
    the `cb_get_clustername()` callback to retrieve the value from the agent with CGO. If
    the callback has not been set `None` will be returned.
*/
PyObject *get_clustername(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_get_clustername == NULL) {
        Py_RETURN_NONE;
    }

    char *v = NULL;
    cb_get_clustername(&v);

    if (v != NULL) {
        PyObject *retval = PyUnicode_FromString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/*! \fn PyObject *tracemalloc_enabled(PyObject *self, PyObject *args)
    \brief This function implements the `datadog-agent.tracemalloc_enabled` method, returning
    whether or not tracemalloc is enabled
    \return a PyObject * pointer to Py_True or Py_False with the state of tracemalloc.

    This function is callable as the `datadog_agent.tracemalloc_enabled` python
    method, it uses the `cb_tracemalloc_enabled()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
PyObject *tracemalloc_enabled(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_tracemalloc_enabled == NULL) {
        Py_RETURN_FALSE;
    }

    bool enabled = cb_tracemalloc_enabled();

    if (enabled) {
        Py_RETURN_TRUE;
    }

    Py_RETURN_FALSE;
}

/*! \fn PyObject *log_message(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.log` method, allowing to log
    python messages using the agent's go logging subsytem and its facilities.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the canonical clustername. Or
    `None` if the callback is unavailable.

    This function is callable as the `datadog_agent.log` python method, it calls back
    into the agent via the `agent_log()` and its `cb_log()` callback. This allows us to use
    the agent logging facilities from python-land.
    Should the callback not be available the function will do nothing.
*/
static PyObject *log_message(PyObject *self, PyObject *args)
{
    char *message = NULL;
    int log_level;

    PyGILState_STATE gstate = PyGILState_Ensure();

    // PyArg_ParseTuple returns a pointer to the existing string in &message
    // No need to free the result.
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    PyGILState_Release(gstate);

    agent_log(log_level, message);
    Py_RETURN_NONE;
}

/*! \fn PyObject *send_log(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.send_log` method, sending
    a log for eventual submission.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a 2-ary tuple containing a log line and the
    unique ID of a check instance.
    \return A PyObject* pointer to `None`.

    This function is callable as the `datadog_agent.send_log` Python method and
    uses the `cb_send_log()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *send_log(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_send_log == NULL) {
        Py_RETURN_NONE;
    }

    char *log_line, *check_id;

    PyGILState_STATE gstate = PyGILState_Ensure();

    // datadog_agent.send_log(log_line, check_id)
    if (!PyArg_ParseTuple(args, "ss", &log_line, &check_id)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    PyGILState_Release(gstate);
    cb_send_log(log_line, check_id);

    Py_RETURN_NONE;
}

/*! \fn PyObject *set_check_metadata(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.set_check_metadata` method, updating
    the value in the cache.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a 3-ary tuple containing the unique ID of a check
    instance, the name of the metadata entry, and the value of said entry.
    \return A PyObject* pointer to `None`.

    This function is callable as the `datadog_agent.set_check_metadata` Python method and
    uses the `cb_set_check_metadata()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *set_check_metadata(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_set_check_metadata == NULL) {
        Py_RETURN_NONE;
    }

    char *check_id, *name, *value;

    PyGILState_STATE gstate = PyGILState_Ensure();

    // datadog_agent.set_check_metadata(check_id, name, value)
    if (!PyArg_ParseTuple(args, "sss", &check_id, &name, &value)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    PyGILState_Release(gstate);
    cb_set_check_metadata(check_id, name, value);

    Py_RETURN_NONE;
}

/*! \fn PyObject *write_persistent_cache(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.write_persistent_cache` method, storing
    the value for the key.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a 2-ary tuple containing the key and the value to store.
    \return A PyObject* pointer to `None`.

    This function is callable as the `datadog_agent.write_persistent_cache` Python method and
    uses the `cb_write_persistent_cache()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *write_persistent_cache(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_write_persistent_cache == NULL) {
        Py_RETURN_NONE;
    }

    char *key, *value;

    // datadog_agent.write_persistent_cache(key, value)
    if (!PyArg_ParseTuple(args, "ss", &key, &value)) {
        return NULL;
    }

    Py_BEGIN_ALLOW_THREADS
    cb_write_persistent_cache(key, value);
    Py_END_ALLOW_THREADS

    Py_RETURN_NONE;
}

/*! \fn PyObject *read_persistent_cache(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.read_persistent_cache` method, retrieving
    the value for the key previously stored.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing the key to retrieve.
    \return A PyObject* pointer to the value.

    This function is callable as the `datadog_agent.read_persistent_cache` Python method and
    uses the `cb_read_persistent_cache()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *read_persistent_cache(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_read_persistent_cache == NULL) {
        Py_RETURN_NONE;
    }

    char *key;

    // datadog_agent.read_persistent_cache(key)
    if (!PyArg_ParseTuple(args, "s", &key)) {
        return NULL;
    }

    char *v = NULL;
    Py_BEGIN_ALLOW_THREADS
    v = cb_read_persistent_cache(key);
    Py_END_ALLOW_THREADS

    if (v == NULL) {
        PyErr_SetString(PyExc_RuntimeError, "failed to read data");
        return NULL;
    }

    PyObject *retval = PyUnicode_FromString(v);
    cgo_free(v);
    return retval;
}

/*! \fn PyObject *set_external_tags(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.set_external_tags` method,
    allowing to set additional external tags for hostnames.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing a single positional argument
    containing a list.
    \return a PyObject * pointer to `None` if everything goes well, or `NULL` if an exception
    is raised.

    This function is callable as the `datadog_agent.set_external_tags` python method, it uses
    the `cb_set_external_tags()` callback to set additional external tags for specific hostnames.
    The argument expected is a list of 2-tuples, where the first element is the hostname, and
    the second element is a dictionary with `source_type` as the key, and a list of tags for
    said `source_type`. For instance: `[('hostname', {'source_type': ['tag1', 'tag2']})]`.
    This function will iterate the python list, and call the `cb_set_external_tags` successively
    for each element in the list.
    If everything goes well `None` will be returned, otherwise an exception will be set in the
    interpreter and NULL will be returned.

    A few integrations such as vsphere or openstack require this functionality to add additional
    tagging for their hosts.
*/
static PyObject *set_external_tags(PyObject *self, PyObject *args)
{
    PyObject *input_list = NULL;

    // callback must be set
    if (cb_set_external_tags == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    // function expects only one positional arg containing a list
    // the reference count in the returned object (input list) is _not_
    // incremented
    if (!PyArg_ParseTuple(args, "O", &input_list)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    // if not a list, set an error
    if (!PyList_Check(input_list)) {
        PyErr_SetString(PyExc_TypeError, "tags must be a list");
        PyGILState_Release(gstate);
        return NULL;
    }

    int error = 0;
    char *hostname = NULL;
    char *source_type = NULL;
    // We already PyList_Check input_list, so PyList_Size won't fail and return -1
    int input_len = PyList_Size(input_list);
    int i;
    for (i = 0; i < input_len; i++) {
        PyObject *tuple = PyList_GetItem(input_list, i);

        // list must contain only tuples in form ('hostname', {'source_type': ['tag1', 'tag2']},)
        if (!PyTuple_Check(tuple)) {
            PyErr_SetString(PyExc_TypeError, "external host tags list must contain only tuples");
            error = 1;
            goto done;
        }

        // first elem is the hostname
        hostname = as_string(PyTuple_GetItem(tuple, 0));
        if (hostname == NULL) {
            PyErr_SetString(PyExc_TypeError, "hostname is not a valid string");
            error = 1;
            goto done;
        }

        // second is a dictionary
        PyObject *dict = PyTuple_GetItem(tuple, 1);
        if (!PyDict_Check(dict)) {
            PyErr_SetString(PyExc_TypeError, "second elem of the host tags tuple must be a dict");
            error = 1;
            goto done;
        }

        // dict contains only 1 key, if dict is empty don't do anything
        Py_ssize_t pos = 0;
        PyObject *key = NULL, *value = NULL;
        if (!PyDict_Next(dict, &pos, &key, &value)) {
            _free(hostname);
            hostname = NULL;
            continue;
        }

        // key is the source type (e.g. 'vsphere') value is the list of tags
        source_type = as_string(key);
        if (source_type == NULL) {
            PyErr_SetString(PyExc_TypeError, "source_type is not a valid string");
            error = 1;
            goto done;
        }

        if (!PyList_Check(value)) {
            PyErr_SetString(PyExc_TypeError, "dict value must be a list of tags");
            error = 1;
            goto done;
        }

        // allocate an array of char* to store the tags we'll send to the Go function
        char **tags;
        // We already PyList_Check value, so PyList_Size won't fail and return -1
        int tags_len = PyList_Size(value);
        if (!(tags = (char **)_malloc(sizeof(*tags) * tags_len + 1))) {
            PyErr_SetString(PyExc_MemoryError, "unable to allocate memory, bailing out");
            error = 1;
            goto done;
        }

        // copy the list of tags into an array of char*
        int j, actual_size = 0;
        for (j = 0; j < tags_len; j++) {
            PyObject *s = PyList_GetItem(value, j);
            if (s == NULL) {
                PyErr_Clear();
                break;
            }

            char *tag = as_string(s);
            if (tag == NULL) {
                // ignore invalid tag
                continue;
            }

            tags[actual_size] = tag;
            actual_size++;
        }
        tags[actual_size] = NULL;

        cb_set_external_tags(hostname, source_type, tags);

        // cleanup
        for (j = 0; j < actual_size; j++) {
            _free(tags[j]);
        }
        _free(tags);

        if (hostname) {
            _free(hostname);
            hostname = NULL;
        }
        if (source_type) {
            _free(source_type);
            source_type = NULL;
        }
    }

done:
    if (hostname) {
        _free(hostname);
    }
    if (source_type) {
        _free(source_type);
    }
    PyGILState_Release(gstate);

    // we need to return NULL to raise the exception set by PyErr_SetString
    if (error) {
        return NULL;
    }
    Py_RETURN_NONE;

}

/*! \fn PyObject *obfuscate_sql(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function implements the `datadog_agent.obfuscate_sql` method, obfuscating
    the provided sql string.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing the key to retrieve.
    \param kwargs A PyObject* pointer to a map of key value pairs.
    \return A PyObject* pointer to the value.

    This function is callable as the `datadog_agent.obfuscate_sql` Python method and
    uses the `cb_obfuscate_sql()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *obfuscate_sql(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set
    if (cb_obfuscate_sql == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    char *rawQuery = NULL;
    char *optionsObj = NULL;
    static char *kwlist[] = {"query", "options", NULL};
    if (!PyArg_ParseTupleAndKeywords(args, kwargs, "s|s", kwlist, &rawQuery, &optionsObj)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    char *obfQuery = NULL;
    char *error_message = NULL;
    obfQuery = cb_obfuscate_sql(rawQuery, optionsObj, &error_message);

    PyObject *retval = NULL;
    if (error_message != NULL) {
        PyErr_SetString(PyExc_RuntimeError, error_message);
    } else if (obfQuery == NULL) {
        // no error message and a null response. this should never happen so the go code is misbehaving
        PyErr_SetString(PyExc_RuntimeError, "internal error: empty cb_obfuscate_sql response");
    } else {
        retval = PyUnicode_FromString(obfQuery);
    }

    cgo_free(error_message);
    cgo_free(obfQuery);
    PyGILState_Release(gstate);
    return retval;
}

static PyObject *obfuscate_sql_exec_plan(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set
    if (cb_obfuscate_sql_exec_plan == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    char *rawPlan = NULL;
    PyObject *normalizeObj = NULL;
    static char *kwlist[] = {"", "normalize", NULL};
    if (!PyArg_ParseTupleAndKeywords(args, kwargs, "s|O", kwlist, &rawPlan, &normalizeObj)) {
        PyGILState_Release(gstate);
        return NULL;
    }
    bool normalize = (normalizeObj != NULL && PyBool_Check(normalizeObj) && normalizeObj == Py_True);

    char *error_message = NULL;
    char *obfPlan = cb_obfuscate_sql_exec_plan(rawPlan, normalize, &error_message);

    PyObject *retval = NULL;
    if (error_message != NULL) {
        PyErr_SetString(PyExc_RuntimeError, error_message);
    } else if (obfPlan == NULL) {
        // no error message and a null response. this should never happen so the go code is misbehaving
        PyErr_SetString(PyExc_RuntimeError, "internal error: empty cb_obfuscate_sql_exec_plan response");
    } else {
        retval = PyUnicode_FromString(obfPlan);
    }

    cgo_free(error_message);
    cgo_free(obfPlan);
    PyGILState_Release(gstate);
    return retval;
}

/*! \fn PyObject *get_process_start_time(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function implements the `datadog_agent.get_process_start_time` method, returning
    the agent process start time, in seconds since the epoch.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to the arguments tuple.
    \param kwargs A PyObject* pointer to a map of key value pairs.
    \return A PyObject* pointer to the value.

    This function is callable as the `datadog_agent.get_process_start_time` Python method and
    uses the `cb_get_process_start_time()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *get_process_start_time(PyObject *self, PyObject *args, PyObject *kwargs)
{
    if (cb_get_process_start_time == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    double time = cb_get_process_start_time();
    PyObject *retval = PyFloat_FromDouble(time);

    PyGILState_Release(gstate);

    return retval;
}

/*! \fn PyObject *obfuscate_mongodb_string(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function implements the `datadog_agent.obfuscate_mongodb_string` method, obfuscating
    the provided mongodb command string.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing the key to retrieve.
    \param kwargs A PyObject* pointer to a map of key value pairs.
    \return A PyObject* pointer to the value.

    This function is callable as the `datadog_agent.obfuscate_mongodb_string` Python method and
    uses the `cb_obfuscate_mongodb_string()` callback to retrieve the value from the agent
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *obfuscate_mongodb_string(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set
    if (cb_obfuscate_mongodb_string == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    char *cmd = NULL;
    if (!PyArg_ParseTuple(args, "s", &cmd)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    char *obfCmd = NULL;
    char *error_message = NULL;
    obfCmd = cb_obfuscate_mongodb_string(cmd, &error_message);

    PyObject *retval = NULL;
    if (error_message != NULL) {
        PyErr_SetString(PyExc_RuntimeError, error_message);
    } else if (obfCmd == NULL) {
        // no error message and a null response. this should never happen so the go code is misbehaving
        PyErr_SetString(PyExc_RuntimeError, "internal error: empty cb_obfuscate_mongodb_string response");
    } else {
        retval = PyUnicode_FromString(obfCmd);
    }

    cgo_free(error_message);
    cgo_free(obfCmd);
    PyGILState_Release(gstate);
    return retval;
}

/*! \fn PyObject *emit_agent_telemetry(PyObject *self, PyObject *args, PyObject *kwargs)
    \brief This function implements the `datadog_agent.emit_agent_telemetry` method, emitting agent telemetry
    for the provided check, metric, and value.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to a tuple containing the key to retrieve.
    \param kwargs A PyObject* pointer to a map of key value pairs.
    \return A PyObject* pointer to the value.

    This function is callable as the `datadog_agent.emit_agent_telemetry` Python method and
    uses the `cb_emit_agent_telemetry()` callback to emit the agent telemetry
    with CGO. If the callback has not been set `None` will be returned.
*/
static PyObject *emit_agent_telemetry(PyObject *self, PyObject *args, PyObject *kwargs)
{
    // callback must be set
    if (cb_emit_agent_telemetry == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    char *check_name = NULL;
    char *metric_name = NULL;
    double metric_value;
    char *metric_type = NULL;
    if (!PyArg_ParseTuple(args, "ssds", &check_name, &metric_name, &metric_value, &metric_type)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    cb_emit_agent_telemetry(check_name, metric_name, metric_value, metric_type);

    PyGILState_Release(gstate);

    Py_RETURN_NONE;
}
