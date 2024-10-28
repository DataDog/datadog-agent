// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_THREE_AGGREGATOR_H
#define DATADOG_AGENT_RTLOADER_THREE_AGGREGATOR_H

/*! \file aggregator.h
    \brief RtLoader Aggregator builtin header file.

    The prototypes here defined provide functions to initialize the python aggregator
    builtin module, and set relevant callbacks in the context of the aggregator for
    metrics, events and service_checks.
*/
/*! \def AGGREGATOR_MODULE_NAME
    \brief Aggregator module name definition..
*/
/*! \fn PyInit_aggregator()
    \brief a function to initialize the python aggregator module in python3.
    \return a pyobject * pointer to the aggregator module.

    This function is only available when python3 is enabled.
*/
/*! \fn void _set_submit_metric_cb(cb_submit_metric_t)
    \brief Sets the submit metric callback to be used by rtloader for metric submission.
    \param cb A function pointer with cb_submit_metric_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_service_check_cb(cb_submit_service_check_t)
    \brief Sets the submit service_check callback to be used by rtloader for service_check
    submission.
    \param cb A function pointer with cb_submit_service_check_t prototype to the
    callback function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_event_cb(cb_submit_event_t)
    \brief Sets the submit event callback to be used by rtloader for event submission.
    \param cb A function pointer with cb_submit_event_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_histogram_bucket_cb(cb_submit_histogram_bucket_t)
    \brief Sets the submit event callback to be used by rtloader for histogram bucket submission.
    \param cb A function pointer with cb_submit_histogram_bucket_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/
/*! \fn void _set_submit_event_platform_event_cb(cb_submit_event_platform_event_t)
    \brief Sets the submit event callback to be used by rtloader for event-platform event submission.
    \param cb A function pointer with cb_submit_event_platform_event_t prototype to the callback
    function.

    The callback is expected to be provided by the rtloader caller - in go-context: CGO.
*/

#define PY_SSIZE_T_CLEAN
#include <Python.h>
#include <rtloader_types.h>

#define AGGREGATOR_MODULE_NAME "aggregator"

#ifdef __cplusplus
extern "C" {
#endif

// PyMODINIT_FUNC macro already specifies extern "C", nesting these is legal
PyMODINIT_FUNC PyInit_aggregator(void);

void _set_submit_metric_cb(cb_submit_metric_t cb);
void _set_submit_service_check_cb(cb_submit_service_check_t cb);
void _set_submit_event_cb(cb_submit_event_t cb);
void _set_submit_histogram_bucket_cb(cb_submit_histogram_bucket_t cb);
void _set_submit_event_platform_event_cb(cb_submit_event_platform_event_t cb);

#ifdef __cplusplus
}
#endif

#endif
