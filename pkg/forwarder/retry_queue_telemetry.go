// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import "expvar"

type retryQueueTelemetryExpVarData struct {
	removalPolicy               expvar.Map
	newRemovalPolicyCount       expvar.Int
	registeredDomainCount       expvar.Int
	outdatedFilesCount          expvar.Int
	filesFromUnknownDomainCount expvar.Int

	transactionContainer     expvar.Map
	currentMemSizeInBytes    expvar.Int
	transactionsCount        expvar.Int
	transactionsDroppedCount expvar.Int
	errorsCount              expvar.Int

	fileStorageCount        expvar.Map
	serializeCount          expvar.Int
	deserializeCount        expvar.Int
	fileSize                expvar.Int
	currentSizeInBytes      expvar.Int
	filesCount              expvar.Int
	reloadedRetryFilesCount expvar.Int
	filesRemovedCount       expvar.Int
}

var (
	retryQueueTelemetryExpVar = retryQueueTelemetryExpVarData{}
)

func init() {
	expVars := &retryQueueTelemetryExpVar

	removalPolicy := &expVars.removalPolicy
	forwarderExpvars.Set("RemovalPolicy", removalPolicy)
	removalPolicy.Set("NewRemovalPolicyCount", &expVars.newRemovalPolicyCount)
	removalPolicy.Set("RegisteredDomainCount", &expVars.registeredDomainCount)
	removalPolicy.Set("OutdatedFilesCount", &expVars.outdatedFilesCount)
	removalPolicy.Set("FilesFromUnknownDomainCount", &expVars.filesFromUnknownDomainCount)

	transactionContainer := &expVars.transactionContainer
	forwarderExpvars.Set("TransactionContainer", transactionContainer)
	transactionContainer.Set("CurrentMemSizeInBytes", &expVars.currentMemSizeInBytes)
	transactionContainer.Set("TransactionsCount", &expVars.transactionsCount)
	transactionContainer.Set("TransactionsDroppedCount", &expVars.transactionsDroppedCount)
	transactionContainer.Set("ErrorsCount", &expVars.errorsCount)

	fileStorage := &expVars.fileStorageCount
	forwarderExpvars.Set("FileStorage", fileStorage)
	fileStorage.Set("SerializeCount", &expVars.serializeCount)
	fileStorage.Set("DeserializeCount", &expVars.deserializeCount)
	fileStorage.Set("FileSize", &expVars.fileSize)
	fileStorage.Set("CurrentSizeInBytes", &expVars.currentSizeInBytes)
	fileStorage.Set("FilesCount", &expVars.filesCount)
	fileStorage.Set("ReloadedRetryFilesCount", &expVars.reloadedRetryFilesCount)
	fileStorage.Set("FilesRemovedCount", &expVars.filesRemovedCount)
}

type failedTransactionRemovalPolicyTelemetry struct{}

func (failedTransactionRemovalPolicyTelemetry) addNewRemovalPolicyCount() {
	retryQueueTelemetryExpVar.newRemovalPolicyCount.Add(1)
}

func (failedTransactionRemovalPolicyTelemetry) addRegisteredDomainCount() {
	retryQueueTelemetryExpVar.registeredDomainCount.Add(1)
}
func (failedTransactionRemovalPolicyTelemetry) addOutdatedFilesCount(count int) {
	retryQueueTelemetryExpVar.outdatedFilesCount.Add(int64(count))
}

func (failedTransactionRemovalPolicyTelemetry) addFilesFromUnknownDomainCount(count int) {
	retryQueueTelemetryExpVar.filesFromUnknownDomainCount.Add(int64(count))
}

type transactionContainerTelemetry struct{}

func (transactionContainerTelemetry) setCurrentMemSizeInBytes(count int) {
	retryQueueTelemetryExpVar.currentMemSizeInBytes.Set(int64(count))
}

func (transactionContainerTelemetry) setTransactionsCount(count int) {
	retryQueueTelemetryExpVar.transactionsCount.Set(int64(count))
}

func (transactionContainerTelemetry) addTransactionsDroppedCount(count int) {
	retryQueueTelemetryExpVar.transactionsDroppedCount.Add(int64(count))
}

func (transactionContainerTelemetry) incErrorsCount() {
	retryQueueTelemetryExpVar.errorsCount.Add(1)
}

type transactionsFileStorageTelemetry struct{}

func (transactionsFileStorageTelemetry) addSerializeCount() {
	retryQueueTelemetryExpVar.serializeCount.Add(1)
}

func (transactionsFileStorageTelemetry) addDeserializeCount() {
	retryQueueTelemetryExpVar.deserializeCount.Add(1)
}

func (transactionsFileStorageTelemetry) setFileSize(count int64) {
	retryQueueTelemetryExpVar.fileSize.Set(count)
}

func (transactionsFileStorageTelemetry) setCurrentSizeInBytes(count int64) {
	retryQueueTelemetryExpVar.currentSizeInBytes.Set(count)
}
func (transactionsFileStorageTelemetry) setFilesCount(count int) {
	retryQueueTelemetryExpVar.filesCount.Set(int64(count))
}

func (transactionsFileStorageTelemetry) addReloadedRetryFilesCount(count int) {
	retryQueueTelemetryExpVar.reloadedRetryFilesCount.Add(int64(count))
}

func (transactionsFileStorageTelemetry) addFilesRemovedCount() {
	retryQueueTelemetryExpVar.filesRemovedCount.Add(1)
}
