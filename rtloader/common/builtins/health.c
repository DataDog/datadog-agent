// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState
#include "health.h"
#include "rtloader_mem.h"
#include "stringutils.h"

// these must be set by the Agent
static cb_submit_health_check_data_t cb_submit_health_check_data = NULL;
static cb_submit_health_start_snapshot_t cb_submit_health_start_snapshot = NULL;
static cb_submit_health_stop_snapshot_t cb_submit_health_stop_snapshot = NULL;

// forward declarations
static PyObject *submit_health_check_data(PyObject *self, PyObject *args);
static PyObject *submit_health_start_snapshot(PyObject *self, PyObject *args);
static PyObject *submit_health_stop_snapshot(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    {"submit_health_check_data", (PyCFunction)submit_health_check_data, METH_VARARGS, "Submit health check data to the health api."},
    {"submit_health_start_snapshot", (PyCFunction)submit_health_start_snapshot, METH_VARARGS, "Submit a health snapshot start to the health api."},
    {"submit_health_stop_snapshot", (PyCFunction)submit_health_stop_snapshot, METH_VARARGS, "Submit a health snapshot stop to the health api."},
    {NULL, NULL}  // guards
};


#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, HEALTH_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_health(void)
{
    return PyModule_Create(&module_def);
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_health()
{
    module = Py_InitModule(HEALTH_MODULE_NAME, methods);
}
#endif


void _set_submit_health_check_data_cb(cb_submit_health_check_data_t cb)
{
    cb_submit_health_check_data = cb;
}

void _set_submit_health_start_snapshot_cb(cb_submit_health_start_snapshot_t cb)
{
    cb_submit_health_start_snapshot = cb;
}

void _set_submit_health_stop_snapshot_cb(cb_submit_health_stop_snapshot_t cb)
{
    cb_submit_health_stop_snapshot = cb;
}


/*! \fn submit_health_check_data(PyObject *self, PyObject *args)
    \brief Health builtin class method for health check data submission.
    \param self A PyObject * pointer to self - the health module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_health_check_data` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit health check data to the batcher.
*/
static PyObject *submit_health_check_data(PyObject *self, PyObject *args) {
    if (cb_submit_health_check_data == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *health_stream_dict = NULL; // borrowed
    PyObject *data_dict = NULL; // borrowed
    char *urn = NULL;
    char *sub_stream = NULL;
    health_stream_t *health_stream_key = NULL;
    char *json_data = NULL;
    PyObject * retval = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOO", &check, &check_id, &health_stream_dict, &data_dict)) {
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(health_stream_dict)) {
        PyErr_SetString(PyExc_TypeError, "health stream must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(data_dict)) {
        PyErr_SetString(PyExc_TypeError, "health check data must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!(health_stream_key = (health_stream_t *)_malloc(sizeof(health_stream_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for health stream key");
        retval = NULL; // Failure
        goto done;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    urn = as_string(PyDict_GetItemString(health_stream_dict, "urn"));
    sub_stream = as_string(PyDict_GetItemString(health_stream_dict, "sub_stream"));
    health_stream_key->urn = urn;
    health_stream_key->sub_stream = sub_stream;

    PyObject *stream = Py_BuildValue("{s:s, s:s}", "urn", urn, "sub_stream", sub_stream);
    PyObject *health = Py_BuildValue("{s:O, s:O}", "stream", stream, "data", data_dict);
    json_data = as_json(health);
    if (json_data == NULL) {
        // If as_json fails it sets a python exception, so we just return
        retval = NULL; // Failure
        goto done;
    } else {
        cb_submit_health_check_data(check_id, health_stream_key, json_data);

        Py_INCREF(Py_None); // Increment, since we are not using the macro Py_RETURN_NONE that does it for us
        retval = Py_None; // Success
    }

done:
    if (health_stream_key != NULL) {
        _free(health_stream_key->urn);
        _free(health_stream_key->sub_stream);
        _free(health_stream_key);
    }
    if (json_data != NULL) {
        _free(json_data);
    }
    PyGILState_Release(gstate);
    return retval;
}

/*! \fn submit_health_start_snapshot(PyObject *self, PyObject *args)
    \brief Health builtin class method to signal the start of a health snapshot submission.
    \param self A PyObject * pointer to self - the health module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_health_start_snapshot` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit the start of health snapshot collection to the batcher.
*/
static PyObject *submit_health_start_snapshot(PyObject *self, PyObject *args) {
    if (cb_submit_health_start_snapshot == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *health_stream_dict = NULL; // borrowed
    int expirySeconds;
    int repeatIntervalSeconds;
    health_stream_t *health_stream_key = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOii", &check, &check_id, &health_stream_dict, &expirySeconds, &repeatIntervalSeconds)) {
      goto error;
    }

    if (!PyDict_Check(health_stream_dict)) {
        PyErr_SetString(PyExc_TypeError, "health stream must be a dict");
        goto error;
    }

    if (!(health_stream_key = (health_stream_t *)_malloc(sizeof(health_stream_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for health stream key");
        goto error;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    health_stream_key->urn = as_string(PyDict_GetItemString(health_stream_dict, "urn"));
    health_stream_key->sub_stream = as_string(PyDict_GetItemString(health_stream_dict, "sub_stream"));

    cb_submit_health_start_snapshot(check_id, health_stream_key, expirySeconds, repeatIntervalSeconds);

    _free(health_stream_key->urn);
    _free(health_stream_key->sub_stream);
    _free(health_stream_key);

    PyGILState_Release(gstate);
    Py_RETURN_NONE; // Success

error:
    PyGILState_Release(gstate);
    return NULL; // Failure
}

/*! \fn submit_health_stop_snapshot(PyObject *self, PyObject *args)
    \brief Health builtin class method to signal the stop of health snapshot submission.
    \param self A PyObject * pointer to self - the health module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_health_stop_snapshot` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit the stop of health collection to the batcher.
*/
static PyObject *submit_health_stop_snapshot(PyObject *self, PyObject *args) {
    if (cb_submit_health_stop_snapshot == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *health_stream_dict = NULL; // borrowed
    health_stream_t *health_stream_key = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &health_stream_dict)) {
      goto error;
    }

    if (!PyDict_Check(health_stream_dict)) {
       PyErr_SetString(PyExc_TypeError, "health stream must be a dict");
       goto error;
   }

   if (!(health_stream_key = (health_stream_t *)_malloc(sizeof(health_stream_t)))) {
       PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for health stream key");
       goto error;
   }

   // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
   health_stream_key->urn = as_string(PyDict_GetItemString(health_stream_dict, "urn"));
   health_stream_key->sub_stream = as_string(PyDict_GetItemString(health_stream_dict, "sub_stream"));

   cb_submit_health_stop_snapshot(check_id, health_stream_key);

   _free(health_stream_key->urn);
   _free(health_stream_key->sub_stream);
   _free(health_stream_key);

   PyGILState_Release(gstate);
   Py_RETURN_NONE; // Success

error:
    PyGILState_Release(gstate);
    return NULL; // Failure
}
