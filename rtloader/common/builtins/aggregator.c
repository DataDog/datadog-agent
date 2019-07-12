// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "aggregator.h"
#include "rtloader_mem.h"
#include "stringutils.h"

// these must be set by the Agent
static cb_submit_metric_t cb_submit_metric = NULL;
static cb_submit_service_check_t cb_submit_service_check = NULL;
static cb_submit_event_t cb_submit_event = NULL;

// forward declarations
static PyObject *submit_metric(PyObject *self, PyObject *args);
static PyObject *submit_service_check(PyObject *self, PyObject *args);
static PyObject *submit_event(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics." },
    { "submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS, "Submit service checks." },
    { "submit_event", (PyCFunction)submit_event, METH_VARARGS, "Submit events." },
    { NULL, NULL } // guards
};

/*! \fn add_constants(PyObject *m)
    \brief A helper function to add a a set of constants to a python module.
    \param m A PyObject * pointer to  the module you wish to add the constant to.

    The returned char ** string array pointer is heap allocated here and should
    be subsequently freed by the caller. This function may set and raise python
    interpreter errors. The function is static and not in the builtin's API.
*/
static void add_constants(PyObject *m)
{
    PyModule_AddIntConstant(m, "GAUGE", DATADOG_AGENT_RTLOADER_GAUGE);
    PyModule_AddIntConstant(m, "RATE", DATADOG_AGENT_RTLOADER_RATE);
    PyModule_AddIntConstant(m, "COUNT", DATADOG_AGENT_RTLOADER_COUNT);
    PyModule_AddIntConstant(m, "MONOTONIC_COUNT", DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT);
    PyModule_AddIntConstant(m, "COUNTER", DATADOG_AGENT_RTLOADER_COUNTER);
    PyModule_AddIntConstant(m, "HISTOGRAM", DATADOG_AGENT_RTLOADER_HISTOGRAM);
    PyModule_AddIntConstant(m, "HISTORATE", DATADOG_AGENT_RTLOADER_HISTORATE);
}

#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, AGGREGATOR_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_aggregator(void)
{
    PyObject *m = PyModule_Create(&module_def);
    add_constants(m);
    return m;
}
#elif defined(DATADOG_AGENT_TWO)
// module object storage
static PyObject *module;

void Py2_init_aggregator()
{
    module = Py_InitModule(AGGREGATOR_MODULE_NAME, methods);
    add_constants(module);
}
#endif

void _set_submit_metric_cb(cb_submit_metric_t cb)
{
    cb_submit_metric = cb;
}

void _set_submit_service_check_cb(cb_submit_service_check_t cb)
{
    cb_submit_service_check = cb;
}

void _set_submit_event_cb(cb_submit_event_t cb)
{
    cb_submit_event = cb;
}

/*! \fn py_tag_to_c(PyObject *py_tags)
    \brief A function to convert a list of python strings (tags) into an
    array of C-strings.
    \return a char ** pointer to the C-representation of the provided python
    tag list. In the event of failure NULL is returned.

    The returned char ** string array pointer is heap allocated here and should
    be subsequently freed by the caller. This function may set and raise python
    interpreter errors. The function is static and not in the builtin's API.
*/
static char **py_tag_to_c(PyObject *py_tags)
{
    char **tags = NULL;
    PyObject *py_tags_list = NULL; // new reference

    if (!PySequence_Check(py_tags)) {
        PyErr_SetString(PyExc_TypeError, "tags must be a sequence");
        return NULL;
    }

    int len = PySequence_Length(py_tags);
    if (len == -1) {
        PyErr_SetString(PyExc_RuntimeError, "could not compute tags length");
        return NULL;
    } else if (len == 0) {
        if (!(tags = _malloc(sizeof(*tags)))) {
            PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for tags");
            return NULL;
        }
        tags[0] = NULL;
        return tags;
    }

    py_tags_list = PySequence_Fast(py_tags, "py_tags is not a sequence"); // new reference
    if (py_tags_list == NULL) {
        goto done;
    }

    if (!(tags = _malloc(sizeof(*tags) * (len + 1)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for tags");
        goto done;
    }
    int nb_valid_tag = 0;
    int i;
    for (i = 0; i < len; i++) {
        // `item` is borrowed, no need to decref
        PyObject *item = PySequence_Fast_GET_ITEM(py_tags_list, i);

        char *ctag = as_string(item);
        if (ctag == NULL) {
            continue;
        }
        tags[nb_valid_tag] = ctag;
        nb_valid_tag++;
    }
    tags[nb_valid_tag] = NULL;

done:
    Py_XDECREF(py_tags_list);
    return tags;
}

/*! \fn free_tags(char **tags)
    \brief A helper function to free the memory allocated by the py_tag_to_c() function.

    This function is for internal use and expects the tag array to be properly intialized,
    and have a NULL canary at the end of the array, just like py_tag_to_c() initializes and
    populates the array. Be mindful if using this function in any other context.
*/
static void free_tags(char **tags)
{
    int i;
    for (i = 0; tags[i] != NULL; i++) {
        _free(tags[i]);
    }
    _free(tags);
}

/*! \fn submit_metric(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method for metric submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_metric` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit metrics to the aggregator.
*/
static PyObject *submit_metric(PyObject *self, PyObject *args)
{
    if (cb_submit_metric == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *name = NULL;
    char *hostname = NULL;
    char *check_id = NULL;
    char **tags = NULL;
    int mt;
    float value;

    // Python call: aggregator.submit_metric(self, check_id, aggregator.metric_type.GAUGE, name, value, tags, hostname)
    if (!PyArg_ParseTuple(args, "OsisfOs", &check, &check_id, &mt, &name, &value, &py_tags, &hostname)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_metric(check_id, mt, name, value, tags, hostname);

    free_tags(tags);

    PyGILState_Release(gstate);
    Py_RETURN_NONE;

error:
    PyGILState_Release(gstate);
    return NULL;
}

/*! \fn submit_service_check(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method for service_check submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_service_check` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit service_checks to the aggregator.
*/
static PyObject *submit_service_check(PyObject *self, PyObject *args)
{
    if (cb_submit_service_check == NULL) {
        Py_RETURN_NONE;
    }

    // acquiring GIL to be able to raise exception
    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *name = NULL;
    int status;
    char *hostname = NULL;
    char *message = NULL;
    char *check_id = NULL;
    char **tags = NULL;

    // aggregator.submit_service_check(self, check_id, name, status, tags, hostname, message)
    if (!PyArg_ParseTuple(args, "OssiOss", &check, &check_id, &name, &status, &py_tags, &hostname, &message)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_service_check(check_id, name, status, tags, hostname, message);

    free_tags(tags);

    PyGILState_Release(gstate);
    Py_RETURN_NONE;

error:
    PyGILState_Release(gstate);
    return NULL;
}

/*! \fn submit_event(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method for event submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_event` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit events to the aggregator.
*/
static PyObject *submit_event(PyObject *self, PyObject *args)
{
    if (cb_submit_event == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *event_dict = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *check_id = NULL;
    event_t *ev = NULL;
    PyObject * retval = NULL;

    // aggregator.submit_event(self, check_id, event)
    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &event_dict)) {
        // error is set by PyArg_ParseTuple but we return NULL to raise
        retval = NULL;
        goto gstate_cleanup;
    }

    if (!PyDict_Check(event_dict)) {
        PyErr_SetString(PyExc_TypeError, "event must be a dict");
        // returning NULL to raise error
        retval = NULL;
        goto gstate_cleanup;
    }

    if (!(ev = (event_t *)_malloc(sizeof(event_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for event");
        retval = NULL;
        goto gstate_cleanup;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    ev->title = as_string(PyDict_GetItemString(event_dict, "msg_title"));
    ev->text = as_string(PyDict_GetItemString(event_dict, "msg_text"));
    // PyLong_AsLong will fail if called passing a NULL argument, be safe
    if (PyDict_GetItemString(event_dict, "timestamp") != NULL) {
        ev->ts = PyLong_AsLong(PyDict_GetItemString(event_dict, "timestamp"));
        if (ev->ts == -1) {
            // we ignore the error and set the timestamp to 0 (magic value that
            // will result in the current time) to ensure backward compatibility
            // with the pre-rtloader API
            PyErr_Clear();
            ev->ts = 0;
        }
    } else {
        ev->ts = 0;
    }
    ev->priority = as_string(PyDict_GetItemString(event_dict, "priority"));
    ev->host = as_string(PyDict_GetItemString(event_dict, "host"));
    ev->alert_type = as_string(PyDict_GetItemString(event_dict, "alert_type"));
    ev->aggregation_key = as_string(PyDict_GetItemString(event_dict, "aggregation_key"));
    ev->source_type_name = as_string(PyDict_GetItemString(event_dict, "source_type_name"));
    ev->event_type = as_string(PyDict_GetItemString(event_dict, "event_type"));
    // process the list of tags, set ev->tags = NULL if tags are missing
    py_tags = PyDict_GetItemString(event_dict, "tags");
    if (py_tags != NULL) {
        ev->tags = py_tag_to_c(py_tags);
        if (ev->tags == NULL) {
            // we need to return NULL to raise the exception set by PyErr_SetString in py_tag_to_c
            retval = NULL;
            goto ev_cleanup;
        }
    } else {
        ev->tags = NULL;
    }

    // send the event
    cb_submit_event(check_id, ev);

    //Success
    Py_INCREF(Py_None); //Increment, sice we are not using the macro Py_RETURN_NONE that does it for us
    retval = Py_None;

ev_cleanup:
    if (ev->tags != NULL) {
        free_tags(ev->tags);
    }
    _free(ev->title);
    _free(ev->text);
    _free(ev->priority);
    _free(ev->host);
    _free(ev->alert_type);
    _free(ev->aggregation_key);
    _free(ev->source_type_name);
    _free(ev->event_type);
    _free(ev);

gstate_cleanup:
    PyGILState_Release(gstate);

    return retval;
}
