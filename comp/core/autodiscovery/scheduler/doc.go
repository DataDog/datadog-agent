// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

/*
Package scheduler is providing the `Scheduler` interface that should be implemented for any scheduler that would want to plug in `autodiscovery`.
It also define the `Controller` which dispatchs all instructions from `autodiscovery` to all the registered schedulers.

Controller
The goal of controller is to transform Autodiscovery to a reconciling controller. Basically the idea is to decouple entirely providers/listeners/resolvers
process from the Check scheduling process.

This Controller component will also serialize (in terms of ordering) the Schedule/Unschedule events and concentrate all the logic to processNextWorkItem,
retry on Unschedule failures, etc.

ConfigState/ConfigStateStore
The ConfigStateStore is a simple in-memory store that keeps track of the desired state of the configs. It is used to keep track of the desired state of the
configs sent from applyChanges
*/
package scheduler
