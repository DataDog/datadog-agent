// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cshared

/*
#cgo CFLAGS: -I../include

#include <stdlib.h>
#include "check_wrapper.h"
#include "sender.h"

// Sender Manager

char *_c_sender_manager_get_sender(sender_manager_t *sender_manager, char * const id, sender_t **ret_sender) {
	return sender_manager->get_sender(sender_manager->handle, id, ret_sender);
}

// Sender

void _c_sender_commit(sender_t *sender) {
	sender->commit(sender->handle);
}

void _c_sender_gauge(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->gauge(sender->handle, metric, value, hostname, tags);
}

void _c_sender_count(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->count(sender->handle, metric, value, hostname, tags);
}

void _c_sender_rate(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->rate(sender->handle, metric, value, hostname, tags);
}

void _c_sender_monotonic_count(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->monotonic_count(sender->handle, metric, value, hostname, tags);
}

void _c_sender_histogram(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->histogram(sender->handle, metric, value, hostname, tags);
}

void _c_sender_historate(sender_t *sender, char *metric, double value, char *hostname, char **tags) {
	sender->historate(sender->handle, metric, value, hostname, tags);
}

void _c_sender_service_check(sender_t *sender, char *service, int status, char *hostname, char **tags, char *message) {
	sender->service_check(sender->handle, service, status, hostname, tags, message);
}

void _c_sender_event_platform_event(sender_t *sender, char *eventType, char *rawEvent) {
	sender->event_platform_event(sender->handle, eventType, rawEvent);
}
*/
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// SENDER MANAGER

var _ sender.SenderManager = (*cSharedSenderManager)(nil)

type cSharedSenderManager struct {
	cSenderManager *C.sender_manager_t
}

func newCSharedSenderManager(cSenderManager *C.sender_manager_t) *cSharedSenderManager {
	return &cSharedSenderManager{cSenderManager: cSenderManager}
}

func (c *cSharedSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	var cSender *C.sender_t
	cId := C.CString(string(id))
	defer C.free(unsafe.Pointer(cId))
	err := C._c_sender_manager_get_sender(c.cSenderManager, cId, &cSender)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return nil, fmt.Errorf("failed to get sender: %s", C.GoString(err))
	}
	return &cSharedSender{cSender: cSender}, nil
}

func (*cSharedSenderManager) SetSender(sender.Sender, checkid.ID) error {
	panic("not implemented")
}

func (*cSharedSenderManager) DestroySender(checkid.ID) {
	panic("not implemented")
}

func (*cSharedSenderManager) GetDefaultSender() (sender.Sender, error) {
	panic("not implemented")
}

// SENDER

var _ sender.Sender = (*cSharedSender)(nil)

type cSharedSender struct {
	cSender *C.sender_t
}

func (*cSharedSender) Commit() {
	C._c_sender_commit(c.cSender)
}

func (*cSharedSender) Gauge(metric string, value float64, hostname string, tags []string) {
	C._c_sender_gauge(c.cSender, C.CString(metric), C.double(value), C.CString(hostname), C.CString(tags))
}

func (*cSharedSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) Rate(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) Count(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	panic("not implemented")
}

func (*cSharedSender) Counter(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) Histogram(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) Historate(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) Distribution(metric string, value float64, hostname string, tags []string) {
	panic("not implemented")
}

func (*cSharedSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	panic("not implemented")
}

func (*cSharedSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool,
	hostname string, tags []string, flushFirstValue bool) {
	panic("not implemented")
}

func (*cSharedSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	panic("not implemented")
}

func (*cSharedSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	panic("not implemented")
}

func (*cSharedSender) Event(e event.Event) {
	panic("not implemented")
}

func (*cSharedSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	panic("not implemented")
}

func (*cSharedSender) GetSenderStats() stats.SenderStats {
	panic("not implemented")
}

func (*cSharedSender) DisableDefaultHostname(disable bool) {
	panic("not implemented")
}

func (*cSharedSender) SetCheckCustomTags(tags []string) {
	panic("not implemented")
}

func (*cSharedSender) SetCheckService(service string) {
	panic("not implemented")
}

func (*cSharedSender) SetNoIndex(noIndex bool) {
	panic("not implemented")
}

func (*cSharedSender) FinalizeCheckServiceTag() {
	panic("not implemented")
}

func (*cSharedSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	panic("not implemented")
}

func (*cSharedSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	panic("not implemented")
}
