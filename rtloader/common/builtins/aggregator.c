// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#include "aggregator.h"
#include "rtloader_mem.h"
#include "stringutils.h"

// these must be set by the Agent
static cb_submit_metric_t cb_submit_metric = NULL;
static cb_submit_service_check_t cb_submit_service_check = NULL;
static cb_submit_event_t cb_submit_event = NULL;
static cb_submit_histogram_bucket_t cb_submit_histogram_bucket = NULL;
static cb_submit_event_platform_event_t cb_submit_event_platform_event = NULL;

// forward declarations
static PyObject *submit_metric(PyObject *self, PyObject *args);
static PyObject *submit_service_check(PyObject *self, PyObject *args);
static PyObject *submit_event(PyObject *self, PyObject *args);
static PyObject *submit_histogram_bucket(PyObject *self, PyObject *args);
static PyObject *submit_event_platform_event(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    { "submit_metric", (PyCFunction)submit_metric, METH_VARARGS, "Submit metrics." },
    { "submit_service_check", (PyCFunction)submit_service_check, METH_VARARGS, "Submit service checks." },
    { "submit_event", (PyCFunction)submit_event, METH_VARARGS, "Submit events." },
    { "submit_histogram_bucket", (PyCFunction)submit_histogram_bucket, METH_VARARGS, "Submit histogram bucket." },
    { "submit_event_platform_event", (PyCFunction)submit_event_platform_event, METH_VARARGS, "Submit event platform event." },
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

/*
 * Sub-interpreter support (Python 3.13+): Multi-phase module initialization
 * =========================================================================
 *
 * Background:
 *   Python sub-interpreters (PEP 684, Python 3.12+) allow running multiple
 *   isolated Python interpreters within a single process. Each sub-interpreter
 *   has its own GIL, sys.modules, and global state - providing true isolation
 *   between Python checks running in the Datadog Agent.
 *
 *   For a C extension module to be importable in sub-interpreters, it must use
 *   "multi-phase initialization" (PEP 489) instead of the legacy "single-phase"
 *   approach. This section converts the aggregator module accordingly.
 *
 * Why multi-phase init is required:
 *   Single-phase init (PyModule_Create with m_size = -1) tells Python: "this
 *   module has unsharable global state and cannot be re-initialized." Python
 *   refuses to import such modules in sub-interpreters when the interpreter is
 *   configured with check_multi_interp_extensions = 1.
 *
 *   Multi-phase init separates module creation (handled by Python internally)
 *   from module population (our "exec" function). Python calls our exec function
 *   once per interpreter, giving each a fresh module object.
 *
 * Why m_size = 0 (no per-interpreter C state):
 *   m_size controls how many bytes of C-level state Python allocates per
 *   interpreter for this module (accessed via PyModule_GetState()).
 *
 *   We set m_size = 0 because our callback pointers (cb_submit_metric, etc.)
 *   are process-global C statics with a "set-once-read-many" access pattern:
 *     - Written exactly once during agent startup (from Go via CGO)
 *     - Read concurrently by any interpreter thereafter
 *     - Never modified or cleared after initialization
 *
 *   This pattern is safe for concurrent reads even with per-interpreter GIL
 *   (no data race on read-only data). Each callback receives a check_id
 *   parameter that routes data to the correct Go-side sender, so the same
 *   function pointer serves all interpreters correctly.
 *
 *   IMPORTANT: If mutable per-interpreter state is ever added to this module,
 *   m_size must be increased to sizeof(that_state_struct) and all functions
 *   must retrieve state via PyModule_GetState() instead of using globals.
 *
 * Py_MOD_PER_INTERPRETER_GIL_SUPPORTED:
 *   This declares that the module is safe to use when each sub-interpreter
 *   has its own GIL (true parallelism). This is a stronger guarantee than
 *   Py_MOD_MULTIPLE_INTERPRETERS_SUPPORTED (which only covers shared-GIL).
 *   Our set-once-read-many callback pattern satisfies this requirement because
 *   concurrent reads of immutable-after-init data need no synchronization.
 *
 * For Python < 3.13, we preserve the original single-phase init unchanged
 * to maintain backward compatibility.
 */
#if PY_VERSION_HEX >= 0x030D0000

/*
 * aggregator_exec: Multi-phase init "exec" slot callback.
 *
 * Called by Python once per interpreter during module import. Receives a
 * freshly-created (empty) module object and populates it with the metric
 * type integer constants (GAUGE, RATE, COUNT, MONOTONIC_COUNT, COUNTER,
 * HISTOGRAM, HISTORATE) that Python check code uses when submitting metrics.
 *
 * Returns 0 on success, -1 with a Python exception set on failure.
 *
 * Error handling note:
 *   add_constants() calls PyModule_AddIntConstant() for each metric type.
 *   PyModule_AddIntConstant() returns -1 and sets a Python exception on
 *   failure (e.g., out of memory), but add_constants() is a legacy void
 *   function that doesn't propagate errors. Rather than changing its
 *   signature (which would affect the legacy init path), we check
 *   PyErr_Occurred() after the call to detect if any constant addition
 *   failed. If so, we return -1 to tell Python the module exec failed,
 *   and Python will propagate the already-set exception.
 */
static int aggregator_exec(PyObject *m)
{
    add_constants(m);

    /* Check if any PyModule_AddIntConstant call inside add_constants failed */
    if (PyErr_Occurred()) {
        return -1;
    }
    return 0;
}

/*
 * Module slot definitions for multi-phase initialization.
 *
 * Py_mod_exec:                     Points to aggregator_exec which populates
 *                                  the module with metric type constants.
 * Py_mod_multiple_interpreters:    Declares per-interpreter GIL support,
 *                                  enabling true parallel execution across
 *                                  sub-interpreters. Safe because all shared
 *                                  C state (callback pointers) is immutable
 *                                  after agent startup.
 */
static PyModuleDef_Slot aggregator_slots[] = {
    {Py_mod_exec, aggregator_exec},
    {Py_mod_multiple_interpreters, Py_MOD_PER_INTERPRETER_GIL_SUPPORTED},
    {0, NULL}  /* sentinel */
};

static struct PyModuleDef module_def = {
    PyModuleDef_HEAD_INIT,
    AGGREGATOR_MODULE_NAME,   /* m_name: "aggregator" */
    NULL,                     /* m_doc */
    0,                        /* m_size: 0 = no per-interpreter C state (see comment above) */
    methods,                  /* m_methods */
    aggregator_slots,         /* m_slots: multi-phase init slot definitions */
    NULL,                     /* m_traverse: no cyclic GC needed */
    NULL,                     /* m_clear: no cyclic GC needed */
    NULL                      /* m_free: no cleanup needed */
};

/*
 * Multi-phase init entry point: returns the PyModuleDef to Python, which
 * handles module object creation internally. Python will then call our
 * aggregator_exec slot to populate the module.
 */
PyMODINIT_FUNC PyInit_aggregator(void)
{
    return PyModuleDef_Init(&module_def);
}

#else /* Python < 3.13: original single-phase initialization */

static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, AGGREGATOR_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_aggregator(void)
{
    PyObject *m = PyModule_Create(&module_def);
    add_constants(m);
    return m;
}

#endif /* PY_VERSION_HEX >= 0x030D0000 */

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

void _set_submit_histogram_bucket_cb(cb_submit_histogram_bucket_t cb)
{
    cb_submit_histogram_bucket = cb;
}

void _set_submit_event_platform_event_cb(cb_submit_event_platform_event_t cb)
{
    cb_submit_event_platform_event = cb;
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
    double value;
    bool flush_first_value = false;

    // Python call: aggregator.submit_metric(self, check_id, aggregator.metric_type.GAUGE, name, value, tags, hostname, flush_first_value)
    if (!PyArg_ParseTuple(args, "OsisdOs|b", &check, &check_id, &mt, &name, &value, &py_tags, &hostname, &flush_first_value)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_metric(check_id, mt, name, value, tags, hostname, flush_first_value);

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

static PyObject *submit_histogram_bucket(PyObject *self, PyObject *args)
{
    if (cb_submit_histogram_bucket == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL; // borrowed
    PyObject *py_tags = NULL; // borrowed
    char *check_id = NULL;
    char *name = NULL;
    long long value;
    float lower_bound;
    float upper_bound;
    int monotonic;
    char *hostname = NULL;
    char **tags = NULL;
    bool flush_first_value = false;

    // Python call: aggregator.submit_histogram_bucket(self, metric string, value, lowerBound, upperBound, monotonic, hostname, tags, flush_first_value)
    if (!PyArg_ParseTuple(args, "OssLffisO|b", &check, &check_id, &name, &value, &lower_bound, &upper_bound, &monotonic, &hostname, &py_tags, &flush_first_value)) {
        goto error;
    }

    if ((tags = py_tag_to_c(py_tags)) == NULL)
        goto error;

    cb_submit_histogram_bucket(check_id, name, value, lower_bound, upper_bound, monotonic, hostname, tags, flush_first_value);

    free_tags(tags);

    PyGILState_Release(gstate);
    Py_RETURN_NONE;

error:
    PyGILState_Release(gstate);
    return NULL;
}

static PyObject *submit_event_platform_event(PyObject *self, PyObject *args)
{
    if (cb_submit_event_platform_event == NULL) {
        Py_RETURN_NONE;
    }

    PyGILState_STATE gstate = PyGILState_Ensure();

    PyObject *check = NULL;
    char *check_id = NULL;
    char *raw_event_ptr = NULL;
    Py_ssize_t raw_event_sz = 0;
    char *event_type = NULL;

    if (!PyArg_ParseTuple(args, "Oss#s", &check, &check_id, &raw_event_ptr, &raw_event_sz, &event_type)) {
        PyGILState_Release(gstate);
        return NULL;
    }

    if (raw_event_sz > INT_MAX) {
        PyErr_SetString(PyExc_ValueError, "event is too large");
        PyGILState_Release(gstate);
        return NULL;
    }

    cb_submit_event_platform_event(check_id, raw_event_ptr, raw_event_sz, event_type);
    PyGILState_Release(gstate);
    Py_RETURN_NONE;
}
