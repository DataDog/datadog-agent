// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#ifndef DATADOG_AGENT_RTLOADER_TAGGER_H
#define DATADOG_AGENT_RTLOADER_TAGGER_H

/*! \file tagger.h
    \brief RtLoader tagger builtin header file.

    The prototypes here defined provide functions to initialize the python tagger
    builtin module, and set its relevant callbacks for the rtloader caller.
*/
/*! \fn PyMODINIT_FUNC PyInit_tagger(void)
    \brief Initializes the tagger builtin python module.

    The tagger python builtin is created and registered here as per the module_def
    PyMethodDef definition. A fresh reference to the module is created here. The
    tag and get_tags methods are registered with the module. This function is python3
    only.
*/
/*! \fn void _set_tags_cb(cb_tags_t)
    \brief Sets a callback to be used by rtloader for setting the relevant tags.
    \param object A function pointer with the cb_tags_t prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
    The callback in turn will call the pertinent internal go-land tagger logic.
    The callback logic will allocate a C(go) pointer array, and the C strings for the
    tagger generate tags. This memory should be freed with the cgo_free helper
    available when done.
*/

#include <Python.h>
#include <rtloader_types.h>

#define TAGGER_MODULE_NAME "tagger"

PyMODINIT_FUNC PyInit_tagger(void);

#ifdef __cplusplus
extern "C" {
#endif

void _set_tags_cb(cb_tags_t);

#ifdef __cplusplus
}
#endif

#endif // DATADOG_AGENT_RTLOADER_TAGGER_H
