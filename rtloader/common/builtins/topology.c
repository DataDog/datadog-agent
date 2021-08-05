// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState
#include "topology.h"
#include "rtloader_mem.h"
#include "stringutils.h"

// these must be set by the Agent
static cb_submit_component_t cb_submit_component = NULL;
static cb_submit_relation_t cb_submit_relation = NULL;
static cb_submit_start_snapshot_t cb_submit_start_snapshot = NULL;
static cb_submit_stop_snapshot_t cb_submit_stop_snapshot = NULL;

// forward declarations
static PyObject *submit_component(PyObject *self, PyObject *args);
static PyObject *submit_relation(PyObject *self, PyObject *args);
static PyObject *submit_start_snapshot(PyObject *self, PyObject *args);
static PyObject *submit_stop_snapshot(PyObject *self, PyObject *args);

static PyMethodDef methods[] = {
    {"submit_component", (PyCFunction)submit_component, METH_VARARGS, "Submit a component to the topology api."},
    {"submit_relation", (PyCFunction)submit_relation, METH_VARARGS, "Submit a relation to the topology api."},
    {"submit_start_snapshot", (PyCFunction)submit_start_snapshot, METH_VARARGS, "Submit a snapshot start to the topology api."},
    {"submit_stop_snapshot", (PyCFunction)submit_stop_snapshot, METH_VARARGS, "Submit a snapshot stop to the topology api."},
    {NULL, NULL}  // guards
};


#ifdef DATADOG_AGENT_THREE
static struct PyModuleDef module_def = { PyModuleDef_HEAD_INIT, TOPOLOGY_MODULE_NAME, NULL, -1, methods };

PyMODINIT_FUNC PyInit_topology(void)
{
    return PyModule_Create(&module_def);
}
#elif defined(DATADOG_AGENT_TWO)
// in Python2 keep the object alive for the program lifetime
static PyObject *module;

void Py2_init_topology()
{
    module = Py_InitModule(TOPOLOGY_MODULE_NAME, methods);
}
#endif


void _set_submit_component_cb(cb_submit_component_t cb)
{
    cb_submit_component = cb;
}

void _set_submit_relation_cb(cb_submit_relation_t cb)
{
    cb_submit_relation = cb;
}

void _set_submit_start_snapshot_cb(cb_submit_start_snapshot_t cb)
{
    cb_submit_start_snapshot = cb;
}

void _set_submit_stop_snapshot_cb(cb_submit_stop_snapshot_t cb)
{
    cb_submit_stop_snapshot = cb;
}


/*! \fn submit_component(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method for topology component submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_component` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit topology component to the aggregator.
*/
static PyObject *submit_component(PyObject *self, PyObject *args) {
    if (cb_submit_component == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *instance_key_dict = NULL; // borrowed
    char *component_id;
    char *component_type;
    PyObject *data_dict = NULL; // borrowed
    instance_key_t *instance_key = NULL;
    char *json_data = NULL;
    PyObject * retval = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOssO", &check, &check_id, &instance_key_dict, &component_id, &component_type, &data_dict)) {
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(instance_key_dict)) {
        PyErr_SetString(PyExc_TypeError, "component instance key must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(data_dict)) {
        PyErr_SetString(PyExc_TypeError, "component data must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!(instance_key = (instance_key_t *)_malloc(sizeof(instance_key_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for topology instance key");
        retval = NULL; // Failure
        goto done;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    instance_key->type_ = as_string(PyDict_GetItemString(instance_key_dict, "type"));
    instance_key->url = as_string(PyDict_GetItemString(instance_key_dict, "url"));

    PyObject *type = Py_BuildValue("{s:s}", "name", component_type);
    PyObject *component = Py_BuildValue("{s:s, s:O, s:O}", "externalId", component_id, "type", type, "data", data_dict);
    json_data = as_json(component);
    if (json_data == NULL) {
        // If as_json fails it sets a python exception, so we just return
        retval = NULL; // Failure
        goto done;
    } else {
        cb_submit_component(check_id, instance_key, component_id, component_type, json_data);

        Py_INCREF(Py_None); // Increment, since we are not using the macro Py_RETURN_NONE that does it for us
        retval = Py_None; // Success
    }

done:
    if (instance_key != NULL) {
        _free(instance_key->type_);
        _free(instance_key->url);
        _free(instance_key);
    }
    if (json_data != NULL) {
        _free(json_data);
    }
    PyGILState_Release(gstate);
    return retval;
}

/*! \fn submit_relation(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method for topology relation submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_relation` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit topology relation to the aggregator.
*/
static PyObject *submit_relation(PyObject *self, PyObject *args) {
    if (cb_submit_relation == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *instance_key_dict = NULL; // borrowed
    char *source_id;
    char *target_id;
    char *relation_type;
    PyObject *data_dict = NULL; // borrowed
    instance_key_t *instance_key = NULL;
    char *json_data = NULL;
    PyObject * retval = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsOsssO", &check, &check_id, &instance_key_dict, &source_id, &target_id, &relation_type, &data_dict)) {
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(instance_key_dict)) {
        PyErr_SetString(PyExc_TypeError, "relation instance key must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!PyDict_Check(data_dict)) {
        PyErr_SetString(PyExc_TypeError, "relation data must be a dict");
        retval = NULL; // Failure
        goto done;
    }

    if (!(instance_key = (instance_key_t *)_malloc(sizeof(instance_key_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for topology instance key");
        retval = NULL; // Failure
        goto done;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    instance_key->type_ = as_string(PyDict_GetItemString(instance_key_dict, "type"));
    instance_key->url = as_string(PyDict_GetItemString(instance_key_dict, "url"));

    PyObject *type = Py_BuildValue("{s:s}", "name", relation_type);
    PyObject *relation = Py_BuildValue("{s:s, s:s, s:O, s:O}", "sourceId", source_id, "targetId", target_id, "type",
        type, "data", data_dict);
    json_data = as_json(relation);
    if (json_data == NULL) {
        // If as_json fails it sets a python exception, so we just return
        retval = NULL; // Failure
        goto done;
    } else {
        cb_submit_relation(check_id, instance_key, source_id, target_id, relation_type, json_data);

        Py_INCREF(Py_None); // Increment, since we are not using the macro Py_RETURN_NONE that does it for us
        retval = Py_None; // Success
    }

done:
    if (instance_key != NULL) {
        _free(instance_key->type_);
        _free(instance_key->url);
        _free(instance_key);
    }
    if (json_data != NULL) {
        _free(json_data);
    }
    PyGILState_Release(gstate);
    return retval;
}

/*! \fn submit_start_snapshot(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method to signal the start of topology submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_start_snapshot` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit the start of topology collection to the aggregator.
*/
static PyObject *submit_start_snapshot(PyObject *self, PyObject *args) {
    if (cb_submit_start_snapshot == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *instance_key_dict = NULL; // borrowed
    instance_key_t *instance_key = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &instance_key_dict)) {
      goto error;
    }

    if (!PyDict_Check(instance_key_dict)) {
        PyErr_SetString(PyExc_TypeError, "relation instance key must be a dict");
        goto error;
    }

    if (!(instance_key = (instance_key_t *)_malloc(sizeof(instance_key_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for topology instance key");
        goto error;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    instance_key->type_ = as_string(PyDict_GetItemString(instance_key_dict, "type"));
    instance_key->url = as_string(PyDict_GetItemString(instance_key_dict, "url"));

    cb_submit_start_snapshot(check_id, instance_key);

    _free(instance_key->type_);
    _free(instance_key->url);
    _free(instance_key);

    PyGILState_Release(gstate);
    Py_RETURN_NONE; // Success

error:
    PyGILState_Release(gstate);
    return NULL; // Failure
}

/*! \fn submit_stop_snapshot(PyObject *self, PyObject *args)
    \brief Aggregator builtin class method to signal the stop of topology submission.
    \param self A PyObject * pointer to self - the aggregator module.
    \param args A PyObject * pointer to the python args or kwargs.
    \return This function returns a new reference to None (already INCREF'd), or NULL in case of error.

    This function implements the `submit_stop_snapshot` python callable in C and is used from the python code.
    More specifically, in the context of rtloader and datadog-agent, this is called from our python base check
    class to submit the stop of topology collection to the aggregator.
*/
static PyObject *submit_stop_snapshot(PyObject *self, PyObject *args) {
    if (cb_submit_stop_snapshot == NULL) {
        Py_RETURN_NONE;
    }

    PyObject *check = NULL; // borrowed
    char *check_id;
    PyObject *instance_key_dict = NULL; // borrowed
    instance_key_t *instance_key = NULL;

    PyGILState_STATE gstate = PyGILState_Ensure();

    if (!PyArg_ParseTuple(args, "OsO", &check, &check_id, &instance_key_dict)) {
      goto error;
    }

    if (!PyDict_Check(instance_key_dict)) {
        PyErr_SetString(PyExc_TypeError, "relation instance key must be a dict");
        goto error;
    }

    if (!(instance_key = (instance_key_t *)_malloc(sizeof(instance_key_t)))) {
        PyErr_SetString(PyExc_RuntimeError, "could not allocate memory for topology instance key");
        goto error;
    }

    // notice: PyDict_GetItemString returns a borrowed ref or NULL if key was not found
    instance_key->type_ = as_string(PyDict_GetItemString(instance_key_dict, "type"));
    instance_key->url = as_string(PyDict_GetItemString(instance_key_dict, "url"));

    cb_submit_stop_snapshot(check_id, instance_key);

    _free(instance_key->type_);
    _free(instance_key->url);
    _free(instance_key);

    PyGILState_Release(gstate);
    Py_RETURN_NONE; // Success

error:
    PyGILState_Release(gstate);
    return NULL; // Failure
}
