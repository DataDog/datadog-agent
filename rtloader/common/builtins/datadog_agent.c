// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "datadog_agent.h"
#include "cgo_free.h"
#include "rtloader_mem.h"
#include "stringutils.h"

// these must be set by the Agent
static cb_get_version_t cb_get_version = NULL;
static cb_get_config_t cb_get_config = NULL;
static cb_headers_t cb_headers = NULL;
static cb_get_hostname_t cb_get_hostname = NULL;
static cb_get_clustername_t cb_get_clustername = NULL;
static cb_log_t cb_log = NULL;
static cb_set_external_tags_t cb_set_external_tags = NULL;

// forward declarations
static PyObject *get_version(PyObject *self, PyObject *args);
static PyObject *get_config(PyObject *self, PyObject *args);
static PyObject *headers(PyObject *self, PyObject *args, PyObject *kwargs);
static PyObject *get_hostname(PyObject *self, PyObject *args);
static PyObject *get_clustername(PyObject *self, PyObject *args);
static PyObject *log_message(PyObject *self, PyObject *args);
static PyObject *set_external_tags(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "get_version", get_version, METH_NOARGS, "Get Agent version." },
    { "get_config", get_config, METH_VARARGS, "Get an Agent config item." },
    { "headers", (PyCFunction)headers, METH_VARARGS | METH_KEYWORDS, "Get standard set of HTTP headers." },
    { "get_hostname", get_hostname, METH_NOARGS, "Get the hostname." },
    { "get_clustername", get_clustername, METH_NOARGS, "Get the cluster name." },
    { "log", log_message, METH_VARARGS, "Log a message through the agent logger." },
    { "set_external_tags", set_external_tags, METH_VARARGS, "Send external host tags." },
    { NULL, NULL } // guards
};

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, DATADOG_AGENT_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_datadog_agent(void)
{
    return PyModule_Create(&module_def);
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_datadog_agent()
{
    module = Py_InitModule(DATADOG_AGENT_MODULE_NAME, methods);
}
#endif

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

void _set_get_clustername_cb(cb_get_clustername_t cb)
{
    cb_get_clustername = cb;
}

void _set_log_cb(cb_log_t cb)
{
    cb_log = cb;
}

void _set_set_external_tags_cb(cb_set_external_tags_t cb)
{
    cb_set_external_tags = cb;
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

    char *v;
    cb_get_version(&v);

    if (v != NULL) {
        PyObject *retval = PyStringFromCString(v);
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
    \brief This function provides a standars set of HTTP headers the caller might want to
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
     2. github.com/DataDog/datadog-agent/blob/master/pkg/util/common.go
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
        PyObject *retval = PyStringFromCString(v);
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
        PyObject *retval = PyStringFromCString(v);
        cgo_free(v);
        return retval;
    }
    Py_RETURN_NONE;
}

/*! \fn PyObject *log_message(PyObject *self, PyObject *args)
    \brief This function implements the `datadog_agent.log` method, allowing to log
    python messages using the agent's go logging subsytem and its facilities.
    \param self A PyObject* pointer to the `datadog_agent` module.
    \param args A PyObject* pointer to any empty tuple, as no input args are taken.
    \return a PyObject * pointer to a python string with the canonical clustername. Or
    `None` if the callback is unavailable.

    This function is callable as the `datadog_agent.get_clustername` python method, it uses
    the `cb_get_clustername()` callback to retrieve the value from the agent with CGO. If
    the callback has not been set `None` will be returned.
*/
static PyObject *log_message(PyObject *self, PyObject *args)
{
    // callback must be set
    if (cb_log == NULL) {
        Py_RETURN_NONE;
    }

    char *message = NULL;
    int log_level;

    // PyArg_ParseTuple returns a pointer to the existing string in &message
    // No need to free the result.
    if (!PyArg_ParseTuple(args, "si", &message, &log_level)) {
        return NULL;
    }

    cb_log(message, log_level);
    Py_RETURN_NONE;
}

//
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
