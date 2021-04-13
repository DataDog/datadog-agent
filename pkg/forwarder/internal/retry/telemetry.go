// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"expvar"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type counterExpvar struct {
	counter telemetry.Counter
	expvar  expvar.Int
}

func newCounterExpvar(subsystem string, name string, help string, parent *expvar.Map) *counterExpvar {
	c := &counterExpvar{
		counter: telemetry.NewCounter(subsystem, name, []string{}, help),
	}
	expvarName := toCamelCase(name)
	parent.Set(expvarName, &c.expvar)
	return c
}

func (c *counterExpvar) add(v float64) {
	c.counter.Add(v)
	c.expvar.Add(int64(v))
}

type gaugeExpvar struct {
	gauge  telemetry.Gauge
	expvar expvar.Int
}

func newGaugeExpvar(subsystem string, name string, help string, parent *expvar.Map) *gaugeExpvar {
	g := &gaugeExpvar{
		gauge: telemetry.NewGauge(subsystem, name, []string{}, help),
	}
	expvarName := toCamelCase(name)
	parent.Set(expvarName, &g.expvar)
	return g
}

func (g *gaugeExpvar) set(v float64) {
	g.gauge.Set(v)
	g.expvar.Set(int64(v))
}

var (
	removalPolicyExpvar                  = expvar.Map{}
	newRemovalPolicyCountTelemetry       *counterExpvar
	registeredDomainCountTelemetry       *counterExpvar
	outdatedFilesCountTelemetry          *counterExpvar
	filesFromUnknownDomainCountTelemetry *counterExpvar

	transactionContainerExpvar        = expvar.Map{}
	currentMemSizeInBytesTelemetry    *gaugeExpvar
	transactionsCountTelemetry        *gaugeExpvar
	transactionsDroppedCountTelemetry *counterExpvar
	errorsCountTelemetry              *counterExpvar

	fileStorageExpvar                     = expvar.Map{}
	serializeCountTelemetry               *counterExpvar
	deserializeCountTelemetry             *counterExpvar
	fileSizeTelemetry                     *gaugeExpvar
	currentSizeInBytesTelemetry           *gaugeExpvar
	filesCountTelemetry                   *gaugeExpvar
	reloadedRetryFilesCountTelemetry      *counterExpvar
	filesRemovedCountTelemetry            *counterExpvar
	deserializeErrorsCountTelemetry       *counterExpvar
	deserializeTransactionsCountTelemetry *counterExpvar
)

func init() {
	transaction.ForwarderExpvars.Set("RemovalPolicy", &removalPolicyExpvar)
	newRemovalPolicyCountTelemetry = newCounterExpvar(
		"removal_policy",
		"new_removal_policy_count",
		"The number of times FileRemovalPolicy is created",
		&removalPolicyExpvar)
	registeredDomainCountTelemetry = newCounterExpvar(
		"removal_policy",
		"registered_domain_count",
		"The number of domains registered by FileRemovalPolicy",
		&removalPolicyExpvar)
	outdatedFilesCountTelemetry = newCounterExpvar(
		"removal_policy",
		"outdated_files_count",
		"The number of outdated files removed",
		&removalPolicyExpvar)
	filesFromUnknownDomainCountTelemetry = newCounterExpvar(
		"removal_policy",
		"files_from_unknown_domain_count",
		"The number of files removed from an unknown domain",
		&removalPolicyExpvar)

	transaction.ForwarderExpvars.Set("TransactionContainer", &transactionContainerExpvar)
	currentMemSizeInBytesTelemetry = newGaugeExpvar(
		"transaction_container",
		"current_mem_size_in_bytes",
		"The retry queue size",
		&transactionContainerExpvar)
	transactionsCountTelemetry = newGaugeExpvar(
		"transaction_container",
		"transactions_count",
		"The number of transactions in the retry queue",
		&transactionContainerExpvar)
	transactionsDroppedCountTelemetry = newCounterExpvar(
		"transaction_container",
		"transactions_dropped_count",
		"The number of transactions dropped because the retry queue is full",
		&transactionContainerExpvar)
	errorsCountTelemetry = newCounterExpvar(
		"transaction_container",
		"errors_count",
		"The number of errors",
		&transactionContainerExpvar)

	transaction.ForwarderExpvars.Set("FileStorage", &fileStorageExpvar)
	serializeCountTelemetry = newCounterExpvar(
		"file_storage",
		"serialize_count",
		"The number of times `transactionsFileStorage.Serialize` is called",
		&fileStorageExpvar)
	deserializeCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_count",
		"The number of times `transactionsFileStorage.Deserialize` is called",
		&fileStorageExpvar)
	fileSizeTelemetry = newGaugeExpvar(
		"file_storage",
		"file_size",
		"The last file size stored on the disk",
		&fileStorageExpvar)
	currentSizeInBytesTelemetry = newGaugeExpvar(
		"file_storage",
		"current_size_in_bytes",
		"The number of bytes used to store transactions on the disk",
		&fileStorageExpvar)
	filesCountTelemetry = newGaugeExpvar(
		"file_storage",
		"files_count",
		"The number of files",
		&fileStorageExpvar)
	reloadedRetryFilesCountTelemetry = newCounterExpvar(
		"file_storage",
		"reloaded_retry_files_count",
		"The number of files reloaded from a previous run of the Agent",
		&fileStorageExpvar)
	filesRemovedCountTelemetry = newCounterExpvar(
		"file_storage",
		"files_removed_count",
		"The number of files removed because the disk limit was reached",
		&fileStorageExpvar)
	deserializeErrorsCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_errors_count",
		"The number of errors during deserialization",
		&fileStorageExpvar)
	deserializeTransactionsCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_transactions_count",
		"The number of transactions read from the disk",
		&fileStorageExpvar)
}

// FileRemovalPolicyTelemetry handles the telemetry for FileRemovalPolicy.
type FileRemovalPolicyTelemetry struct{}

func (FileRemovalPolicyTelemetry) addNewRemovalPolicyCount() {
	newRemovalPolicyCountTelemetry.add(1)
}

func (FileRemovalPolicyTelemetry) addRegisteredDomainCount() {
	registeredDomainCountTelemetry.add(1)
}
func (FileRemovalPolicyTelemetry) addOutdatedFilesCount(count int) {
	outdatedFilesCountTelemetry.add(float64(count))
}

func (FileRemovalPolicyTelemetry) addFilesFromUnknownDomainCount(count int) {
	filesFromUnknownDomainCountTelemetry.add(float64(count))
}

// TransactionRetryQueueTelemetry handles the telemetry for TransactionRetryQueue
type TransactionRetryQueueTelemetry struct{}

func (TransactionRetryQueueTelemetry) setCurrentMemSizeInBytes(count int) {
	currentMemSizeInBytesTelemetry.set(float64(count))
}

func (TransactionRetryQueueTelemetry) setTransactionsCount(count int) {
	transactionsCountTelemetry.set(float64(count))
}

func (TransactionRetryQueueTelemetry) addTransactionsDroppedCount(count int) {
	transactionsDroppedCountTelemetry.add(float64(count))
}

func (TransactionRetryQueueTelemetry) incErrorsCount() {
	errorsCountTelemetry.add(1)
}

type onDiskRetryQueueTelemetry struct{}

func (onDiskRetryQueueTelemetry) addSerializeCount() {
	serializeCountTelemetry.add(1)
}

func (onDiskRetryQueueTelemetry) addDeserializeCount() {
	deserializeCountTelemetry.add(1)
}

func (onDiskRetryQueueTelemetry) setFileSize(count int64) {
	fileSizeTelemetry.set(float64(count))
}

func (onDiskRetryQueueTelemetry) setCurrentSizeInBytes(count int64) {
	currentSizeInBytesTelemetry.set(float64(count))
}

func (onDiskRetryQueueTelemetry) setFilesCount(count int) {
	filesCountTelemetry.set(float64(count))
}

func (onDiskRetryQueueTelemetry) addReloadedRetryFilesCount(count int) {
	reloadedRetryFilesCountTelemetry.add(float64(count))
}

func (onDiskRetryQueueTelemetry) addFilesRemovedCount() {
	filesRemovedCountTelemetry.add(1)
}

func (onDiskRetryQueueTelemetry) addDeserializeErrorsCount(count int) {
	deserializeErrorsCountTelemetry.add(float64(count))
}

func (onDiskRetryQueueTelemetry) addDeserializeTransactionsCount(count int) {
	deserializeTransactionsCountTelemetry.add(float64(count))
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	var camelCase string
	for _, p := range parts {
		camelCase += strings.Title(p)
	}
	return camelCase
}
