// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import "expvar"

var (
	removalPolicyExpvar               = expvar.Map{}
	newRemovalPolicyCountExpvar       = expvar.Int{}
	registeredDomainCountExpvar       = expvar.Int{}
	outdatedFilesCountExpvar          = expvar.Int{}
	filesFromUnknownDomainCountExpvar = expvar.Int{}

	transactionContainerExpvar     = expvar.Map{}
	currentMemSizeInBytesExpvar    = expvar.Int{}
	transactionsCountExpvar        = expvar.Int{}
	transactionsDroppedCountExpvar = expvar.Int{}
	errorsCountExpvar              = expvar.Int{}

	fileStorageExpvar                  = expvar.Map{}
	serializeCountExpvar               = expvar.Int{}
	deserializeCountExpvar             = expvar.Int{}
	fileSizeExpvar                     = expvar.Int{}
	currentSizeInBytesExpvar           = expvar.Int{}
	filesCountExpvar                   = expvar.Int{}
	reloadedRetryFilesCountExpvar      = expvar.Int{}
	filesRemovedCountExpvar            = expvar.Int{}
	deserializeErrorsCountExpvar       = expvar.Int{}
	deserializeTransactionsCountExpvar = expvar.Int{}
)

func init() {
	forwarderExpvars.Set("RemovalPolicy", &removalPolicyExpvar)
	removalPolicyExpvar.Set("NewRemovalPolicyCount", &newRemovalPolicyCountExpvar)
	removalPolicyExpvar.Set("RegisteredDomainCount", &registeredDomainCountExpvar)
	removalPolicyExpvar.Set("OutdatedFilesCount", &outdatedFilesCountExpvar)
	removalPolicyExpvar.Set("FilesFromUnknownDomainCount", &filesFromUnknownDomainCountExpvar)

	forwarderExpvars.Set("TransactionContainer", &transactionContainerExpvar)
	transactionContainerExpvar.Set("CurrentMemSizeInBytes", &currentMemSizeInBytesExpvar)
	transactionContainerExpvar.Set("TransactionsCount", &transactionsCountExpvar)
	transactionContainerExpvar.Set("TransactionsDroppedCount", &transactionsDroppedCountExpvar)
	transactionContainerExpvar.Set("ErrorsCount", &errorsCountExpvar)

	forwarderExpvars.Set("FileStorage", &fileStorageExpvar)
	fileStorageExpvar.Set("SerializeCount", &serializeCountExpvar)
	fileStorageExpvar.Set("DeserializeCount", &deserializeCountExpvar)
	fileStorageExpvar.Set("FileSize", &fileSizeExpvar)
	fileStorageExpvar.Set("CurrentSizeInBytes", &currentSizeInBytesExpvar)
	fileStorageExpvar.Set("FilesCount", &filesCountExpvar)
	fileStorageExpvar.Set("ReloadedRetryFilesCount", &reloadedRetryFilesCountExpvar)
	fileStorageExpvar.Set("FilesRemovedCount", &filesRemovedCountExpvar)
	fileStorageExpvar.Set("DeserializeErrorsCount", &deserializeErrorsCountExpvar)
	fileStorageExpvar.Set("DeserializeTransactionsCount", &deserializeTransactionsCountExpvar)
}

type failedTransactionRemovalPolicyTelemetry struct{}

func (failedTransactionRemovalPolicyTelemetry) addNewRemovalPolicyCount() {
	newRemovalPolicyCountExpvar.Add(1)
}

func (failedTransactionRemovalPolicyTelemetry) addRegisteredDomainCount() {
	registeredDomainCountExpvar.Add(1)
}
func (failedTransactionRemovalPolicyTelemetry) addOutdatedFilesCount(count int) {
	outdatedFilesCountExpvar.Add(int64(count))
}

func (failedTransactionRemovalPolicyTelemetry) addFilesFromUnknownDomainCount(count int) {
	filesFromUnknownDomainCountExpvar.Add(int64(count))
}

type transactionContainerTelemetry struct{}

func (transactionContainerTelemetry) setCurrentMemSizeInBytes(count int) {
	currentMemSizeInBytesExpvar.Set(int64(count))
}

func (transactionContainerTelemetry) setTransactionsCount(count int) {
	transactionsCountExpvar.Set(int64(count))
}

func (transactionContainerTelemetry) addTransactionsDroppedCount(count int) {
	transactionsDroppedCountExpvar.Add(int64(count))
}

func (transactionContainerTelemetry) incErrorsCount() {
	errorsCountExpvar.Add(1)
}

type transactionsFileStorageTelemetry struct{}

func (transactionsFileStorageTelemetry) addSerializeCount() {
	serializeCountExpvar.Add(1)
}

func (transactionsFileStorageTelemetry) addDeserializeCount() {
	deserializeCountExpvar.Add(1)
}

func (transactionsFileStorageTelemetry) setFileSize(count int64) {
	fileSizeExpvar.Set(count)
}

func (transactionsFileStorageTelemetry) setCurrentSizeInBytes(count int64) {
	currentSizeInBytesExpvar.Set(count)
}
func (transactionsFileStorageTelemetry) setFilesCount(count int) {
	filesCountExpvar.Set(int64(count))
}

func (transactionsFileStorageTelemetry) addReloadedRetryFilesCount(count int) {
	reloadedRetryFilesCountExpvar.Add(int64(count))
}

func (transactionsFileStorageTelemetry) addFilesRemovedCount() {
	filesRemovedCountExpvar.Add(1)
}

func (transactionsFileStorageTelemetry) addDeserializeErrorsCount(count int) {
	deserializeErrorsCountExpvar.Add(int64(count))
}

func (transactionsFileStorageTelemetry) addDeserializeTransactionsCount(count int) {
	deserializeTransactionsCountExpvar.Add(int64(count))
}
