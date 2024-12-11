// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"expvar"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type counterExpvar struct {
	counter telemetry.Counter
	expvar  expvar.Int
}

func newCounterExpvar(subsystem string, name string, tags []string, help string, parent *expvar.Map) *counterExpvar {
	c := &counterExpvar{
		counter: telemetry.NewCounter(subsystem, name, tags, help),
	}
	expvarName := toCamelCase(name)
	parent.Set(expvarName, &c.expvar)
	return c
}

func (c *counterExpvar) add(v float64, tagsValue ...string) {
	c.counter.Add(v, tagsValue...)
	c.expvar.Add(int64(v))
}

type gaugeExpvar struct {
	gauge  telemetry.Gauge
	expvar expvar.Int
}

func newGaugeExpvar(subsystem string, name string, tags []string, help string, parent *expvar.Map) *gaugeExpvar {
	g := &gaugeExpvar{
		gauge: telemetry.NewGauge(subsystem, name, tags, help),
	}
	expvarName := toCamelCase(name)
	parent.Set(expvarName, &g.expvar)
	return g
}

func (g *gaugeExpvar) set(v float64, tagsValue ...string) {
	g.gauge.Set(v, tagsValue...)
	g.expvar.Set(int64(v))
}

var (
	removalPolicyExpvar                  = expvar.Map{}
	newRemovalPolicyCountTelemetry       *gaugeExpvar
	registeredDomainCountTelemetry       *gaugeExpvar
	outdatedFilesCountTelemetry          *gaugeExpvar
	filesFromUnknownDomainCountTelemetry *gaugeExpvar

	transactionContainerExpvar        = expvar.Map{}
	currentMemSizeInBytesTelemetry    *gaugeExpvar
	transactionsCountTelemetry        *gaugeExpvar
	transactionsDroppedCountTelemetry *counterExpvar
	errorsCountTelemetry              *counterExpvar

	transactionContainerPointDroppedCountTelemetry *counterExpvar

	fileStorageExpvar                       = expvar.Map{}
	serializeCountTelemetry                 *counterExpvar
	deserializeCountTelemetry               *counterExpvar
	fileSizeTelemetry                       *gaugeExpvar
	currentSizeInBytesTelemetry             *gaugeExpvar
	filesCountTelemetry                     *gaugeExpvar
	startupReloadedRetryFilesCountTelemetry *gaugeExpvar
	filesRemovedCountTelemetry              *counterExpvar
	fileStoragePointDroppedCountTelemetry   *counterExpvar
	deserializeErrorsCountTelemetry         *counterExpvar
	deserializeTransactionsCountTelemetry   *counterExpvar
)

func init() {
	transaction.ForwarderExpvars.Set("RemovalPolicy", &removalPolicyExpvar)
	domainTag := []string{"domain"}
	newRemovalPolicyCountTelemetry = newGaugeExpvar(
		"startup_removal_policy",
		"new_removal_policy_count",
		nil,
		"The number of times FileRemovalPolicy is created",
		&removalPolicyExpvar)
	registeredDomainCountTelemetry = newGaugeExpvar(
		"startup_removal_policy",
		"registered_domain_count",
		domainTag,
		"The number of domains registered by FileRemovalPolicy",
		&removalPolicyExpvar)
	outdatedFilesCountTelemetry = newGaugeExpvar(
		"startup_removal_policy",
		"outdated_files_count",
		nil,
		"The number of outdated files removed",
		&removalPolicyExpvar)
	filesFromUnknownDomainCountTelemetry = newGaugeExpvar(
		"startup_removal_policy",
		"files_from_unknown_domain_count",
		nil,
		"The number of files removed from an unknown domain",
		&removalPolicyExpvar)

	transaction.ForwarderExpvars.Set("TransactionContainer", &transactionContainerExpvar)
	currentMemSizeInBytesTelemetry = newGaugeExpvar(
		"transaction_container",
		"current_mem_size_in_bytes",
		domainTag,
		"The retry queue size",
		&transactionContainerExpvar)
	transactionsCountTelemetry = newGaugeExpvar(
		"transaction_container",
		"transactions_count",
		domainTag,
		"The number of transactions in the retry queue",
		&transactionContainerExpvar)
	transactionsDroppedCountTelemetry = newCounterExpvar(
		"transaction_container",
		"transactions_dropped_count",
		domainTag,
		"The number of transactions dropped because the retry queue is full",
		&transactionContainerExpvar)
	errorsCountTelemetry = newCounterExpvar(
		"transaction_container",
		"errors_count",
		domainTag,
		"The number of errors",
		&transactionContainerExpvar)
	transactionContainerPointDroppedCountTelemetry = newCounterExpvar(
		"transaction_container",
		"points_dropped_count",
		domainTag,
		"The number of points dropped",
		&transactionContainerExpvar)

	transaction.ForwarderExpvars.Set("FileStorage", &fileStorageExpvar)
	serializeCountTelemetry = newCounterExpvar(
		"file_storage",
		"serialize_count",
		domainTag,
		"The number of times `transactionsFileStorage.Serialize` is called",
		&fileStorageExpvar)
	deserializeCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_count",
		domainTag,
		"The number of times `transactionsFileStorage.Deserialize` is called",
		&fileStorageExpvar)
	fileSizeTelemetry = newGaugeExpvar(
		"file_storage",
		"file_size",
		domainTag,
		"The last file size stored on the disk",
		&fileStorageExpvar)
	currentSizeInBytesTelemetry = newGaugeExpvar(
		"file_storage",
		"current_size_in_bytes",
		domainTag,
		"The number of bytes used to store transactions on the disk",
		&fileStorageExpvar)
	filesCountTelemetry = newGaugeExpvar(
		"file_storage",
		"files_count",
		domainTag,
		"The number of files",
		&fileStorageExpvar)
	startupReloadedRetryFilesCountTelemetry = newGaugeExpvar(
		"file_storage",
		"startup_reloaded_retry_files_count",
		domainTag,
		"The number of files reloaded from a previous run of the Agent",
		&fileStorageExpvar)
	filesRemovedCountTelemetry = newCounterExpvar(
		"file_storage",
		"files_removed_count",
		domainTag,
		"The number of files removed because the disk limit was reached",
		&fileStorageExpvar)

	fileStoragePointDroppedCountTelemetry = newCounterExpvar(
		"file_storage",
		"points_dropped_count",
		domainTag,
		"The number of points dropped",
		&fileStorageExpvar)

	deserializeErrorsCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_errors_count",
		domainTag,
		"The number of errors during deserialization",
		&fileStorageExpvar)
	deserializeTransactionsCountTelemetry = newCounterExpvar(
		"file_storage",
		"deserialize_transactions_count",
		domainTag,
		"The number of transactions read from the disk",
		&fileStorageExpvar)
}

// FileRemovalPolicyTelemetry handles the telemetry for FileRemovalPolicy.
type FileRemovalPolicyTelemetry struct{}

func (FileRemovalPolicyTelemetry) setNewRemovalPolicyCount(count int) {
	newRemovalPolicyCountTelemetry.set(float64(count))
}

func (FileRemovalPolicyTelemetry) setRegisteredDomainCount(count int, domainName string) {
	registeredDomainCountTelemetry.set(float64(count), domainName)
}
func (FileRemovalPolicyTelemetry) setOutdatedFilesCount(count int) {
	outdatedFilesCountTelemetry.set(float64(count))
}

func (FileRemovalPolicyTelemetry) setFilesFromUnknownDomainCount(count int) {
	filesFromUnknownDomainCountTelemetry.set(float64(count))
}

// TransactionRetryQueueTelemetry handles the telemetry for TransactionRetryQueue
type TransactionRetryQueueTelemetry struct {
	domainName string
}

// NewTransactionRetryQueueTelemetry creates a new TransactionRetryQueueTelemetry
func NewTransactionRetryQueueTelemetry(domainName string) TransactionRetryQueueTelemetry {
	return TransactionRetryQueueTelemetry{
		domainName: domainName,
	}
}

func (t TransactionRetryQueueTelemetry) setCurrentMemSizeInBytes(count int) {
	currentMemSizeInBytesTelemetry.set(float64(count), t.domainName)
}

func (t TransactionRetryQueueTelemetry) setTransactionsCount(count int) {
	transactionsCountTelemetry.set(float64(count), t.domainName)
}

func (t TransactionRetryQueueTelemetry) addTransactionsDroppedCount(count int) {
	transactionsDroppedCountTelemetry.add(float64(count), t.domainName)
}

func (t TransactionRetryQueueTelemetry) incErrorsCount() {
	errorsCountTelemetry.add(1, t.domainName)
}

func (t TransactionRetryQueueTelemetry) addPointDroppedCount(count int) {
	transactionContainerPointDroppedCountTelemetry.add(float64(count), t.domainName)
}

type onDiskRetryQueueTelemetry struct {
	domainName string
}

func newOnDiskRetryQueueTelemetry(domainName string) onDiskRetryQueueTelemetry {
	return onDiskRetryQueueTelemetry{
		domainName: domainName,
	}
}

func (t onDiskRetryQueueTelemetry) addSerializeCount() {
	serializeCountTelemetry.add(1, t.domainName)
}

func (t onDiskRetryQueueTelemetry) addDeserializeCount() {
	deserializeCountTelemetry.add(1, t.domainName)
}

func (t onDiskRetryQueueTelemetry) setFileSize(count int64) {
	fileSizeTelemetry.set(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) setCurrentSizeInBytes(count int64) {
	currentSizeInBytesTelemetry.set(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) setFilesCount(count int) {
	filesCountTelemetry.set(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) setReloadedRetryFilesCount(count int) {
	startupReloadedRetryFilesCountTelemetry.set(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) addFilesRemovedCount() {
	filesRemovedCountTelemetry.add(1, t.domainName)
}

func (t onDiskRetryQueueTelemetry) addPointDroppedCount(count int) {
	fileStoragePointDroppedCountTelemetry.add(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) addDeserializeErrorsCount(count int) {
	deserializeErrorsCountTelemetry.add(float64(count), t.domainName)
}

func (t onDiskRetryQueueTelemetry) addDeserializeTransactionsCount(count int) {
	deserializeTransactionsCountTelemetry.add(float64(count), t.domainName)
}

func toCamelCase(s string) string {
	caser := cases.Title(language.English)
	parts := strings.Split(s, "_")
	var camelCase string
	for _, p := range parts {
		camelCase += caser.String(p)
	}
	return camelCase
}
