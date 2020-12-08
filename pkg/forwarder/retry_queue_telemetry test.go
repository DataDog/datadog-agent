// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

type retryQueueTelemetryTest struct{}

var _ failedTransactionRemovalPolicyTelemetry = retryQueueTelemetryTest{}

func (retryQueueTelemetryTest) addNewRemovalPolicyCount()                {}
func (retryQueueTelemetryTest) addRegisteredDomainCount()                {}
func (retryQueueTelemetryTest) addOutdatedFilesCount(count int)          {}
func (retryQueueTelemetryTest) addFilesFromUnknownDomainCount(count int) {}

var _ transactionContainerTelemetry = retryQueueTelemetryTest{}

func (retryQueueTelemetryTest) setCurrentMemSizeInBytes(count int)    {}
func (retryQueueTelemetryTest) setTransactionsCount(count int)        {}
func (retryQueueTelemetryTest) addTransactionsDroppedCount(count int) {}
func (retryQueueTelemetryTest) addErrorsCount()                       {}

var _ transactionsFileStorageTelemetry = retryQueueTelemetryTest{}

func (retryQueueTelemetryTest) addSerializeCount()                   {}
func (retryQueueTelemetryTest) addDeserializeCount()                 {}
func (retryQueueTelemetryTest) setFileSize(count int64)              {}
func (retryQueueTelemetryTest) setCurrentSizeInBytes(count int64)    {}
func (retryQueueTelemetryTest) setFilesCount(count int)              {}
func (retryQueueTelemetryTest) addReloadedRetryFilesCount(count int) {}
func (retryQueueTelemetryTest) addFilesRemovedCount()                {}
