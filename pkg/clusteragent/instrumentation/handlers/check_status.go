// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const (
	checkStatusQueueSize      = 128
	checkStatusMaxErrorLength = 1024
)

var errCheckStatusNotLeader = errors.New("cluster agent is not the leader")

// CheckStatusStore aggregates node-side check status transitions and updates
// the originating DatadogInstrumentation status from a dedicated worker.
type CheckStatusStore struct {
	mu      sync.Mutex
	reports map[string]cctypes.InstrumentationCheckStatusReport
	queue   chan string
}

// NewCheckStatusStore creates a runtime status store.
func NewCheckStatusStore() *CheckStatusStore {
	return &CheckStatusStore{
		reports: make(map[string]cctypes.InstrumentationCheckStatusReport),
		queue:   make(chan string, checkStatusQueueSize),
	}
}

// SubmitInstrumentationCheckStatus accepts status transitions from node Agents.
func (s *CheckStatusStore) SubmitInstrumentationCheckStatus(request cctypes.InstrumentationCheckStatusRequest) {
	for _, report := range request.Reports {
		if report.Namespace == "" || report.Name == "" || report.UID == "" || report.Generation < 1 {
			log.Warn("Ignoring incomplete DatadogInstrumentation check status report")
			continue
		}
		if report.State != cctypes.InstrumentationCheckStatusRunning && report.State != cctypes.InstrumentationCheckStatusFailed {
			log.Warnf("Ignoring DatadogInstrumentation check status report with unknown state %q", report.State)
			continue
		}
		if report.Phase != cctypes.InstrumentationCheckStatusConfigure && report.Phase != cctypes.InstrumentationCheckStatusRun {
			log.Warnf("Ignoring DatadogInstrumentation check status report with unknown phase %q", report.Phase)
			continue
		}
		rawError := report.Error
		report.Error, _ = scrubber.ScrubString(rawError)
		if report.Error == "" && rawError != "" {
			report.Error = scrubber.ScrubLine(rawError)
		}
		if len(report.Error) > checkStatusMaxErrorLength {
			report.Error = report.Error[:checkStatusMaxErrorLength]
		}

		s.mu.Lock()
		// Reaching the run phase proves configuration succeeded. Remove a
		// previous configure failure for this instance so recovery only needs
		// the run transition POST.
		if report.Phase == cctypes.InstrumentationCheckStatusRun {
			configureReport := report
			configureReport.Phase = cctypes.InstrumentationCheckStatusConfigure
			delete(s.reports, checkStatusReportKey(configureReport))
		}
		s.reports[checkStatusReportKey(report)] = report
		s.mu.Unlock()
		s.enqueue(report.Namespace + "/" + report.Name)
	}
}

func (s *CheckStatusStore) enqueue(key string) {
	select {
	case s.queue <- key:
	default:
		log.Warnf("Dropping DatadogInstrumentation status reconciliation for %s: queue is full", key)
	}
}

// Run processes runtime status updates until ctx is cancelled.
func (s *CheckStatusStore) Run(ctx context.Context, client dynamic.Interface, isLeader func() bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-s.queue:
			if isLeader != nil && !isLeader() {
				s.requeueAfter(ctx, key, 2*time.Second)
				continue
			}
			if err := s.reconcile(ctx, client, key, isLeader); errors.Is(err, errCheckStatusNotLeader) {
				s.requeueAfter(ctx, key, 2*time.Second)
			} else if err != nil {
				log.Errorf("Unable to update DatadogInstrumentation runtime check status for %s: %v", key, err)
				s.requeueAfter(ctx, key, 10*time.Second)
			}
		}
	}
}

func (s *CheckStatusStore) requeueAfter(ctx context.Context, key string, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		s.enqueue(key)
	}
}

func (s *CheckStatusStore) reconcile(ctx context.Context, client dynamic.Interface, key string, isLeader func() bool) error {
	namespace, name, found := strings.Cut(key, "/")
	if !found {
		return nil
	}
	obj, err := client.Resource(instrumentation.DatadogInstrumentationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cr := &datadoghq.DatadogInstrumentation{}
	if err := instrumentation.UnstructuredIntoDatadogInstrumentation(obj, cr); err != nil {
		return err
	}

	status := aggregateCheckStatus(cr, s.currentReports(cr))
	if checkStatusConditionMatches(cr, status) {
		return nil
	}
	// Leadership may have changed while reading and aggregating the reports.
	// Check again immediately before entering the status write path.
	if isLeader != nil && !isLeader() {
		return errCheckStatusNotLeader
	}
	return instrumentation.UpdateStatusConditions(ctx, client, cr, []instrumentation.HandlerStatus{status})
}

func checkStatusReportKey(report cctypes.InstrumentationCheckStatusReport) string {
	return fmt.Sprintf("%s/%s/%s/%d/%d/%d/%s/%s", report.Namespace, report.Name, report.UID, report.Generation, report.CheckIndex, report.InstanceIndex, report.NodeName, report.Phase)
}

func checkStatusConditionMatches(cr *datadoghq.DatadogInstrumentation, status instrumentation.HandlerStatus) bool {
	condition := meta.FindStatusCondition(cr.Status.Conditions, status.Type)
	return condition != nil &&
		condition.ObservedGeneration == cr.Generation &&
		condition.Status == status.Status &&
		condition.Reason == status.Reason &&
		condition.Message == status.Message
}

func (s *CheckStatusStore) currentReports(cr *datadoghq.DatadogInstrumentation) []cctypes.InstrumentationCheckStatusReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := make([]cctypes.InstrumentationCheckStatusReport, 0)
	for key, report := range s.reports {
		if report.Namespace != cr.Namespace || report.Name != cr.Name {
			continue
		}
		if report.UID != string(cr.UID) || report.Generation != cr.Generation {
			delete(s.reports, key)
			continue
		}
		current = append(current, report)
	}
	return current
}

func aggregateCheckStatus(cr *datadoghq.DatadogInstrumentation, reports []cctypes.InstrumentationCheckStatusReport) instrumentation.HandlerStatus {
	expected := 0
	for _, check := range cr.Spec.Config.Checks {
		expected += len(check.Instances)
	}

	observed := make(map[string]struct{})
	failures := make([]cctypes.InstrumentationCheckStatusReport, 0)
	for _, report := range reports {
		if report.CheckIndex < 0 || report.CheckIndex >= len(cr.Spec.Config.Checks) || report.InstanceIndex < 0 || report.InstanceIndex >= len(cr.Spec.Config.Checks[report.CheckIndex].Instances) {
			continue
		}
		if report.Phase == cctypes.InstrumentationCheckStatusRun && report.State == cctypes.InstrumentationCheckStatusRunning {
			observed[fmt.Sprintf("%d/%d", report.CheckIndex, report.InstanceIndex)] = struct{}{}
		}
		if report.State == cctypes.InstrumentationCheckStatusFailed {
			failures = append(failures, report)
		}
	}
	if len(failures) > 0 {
		sort.Slice(failures, func(i, j int) bool {
			if failures[i].CheckIndex == failures[j].CheckIndex {
				return failures[i].InstanceIndex < failures[j].InstanceIndex
			}
			return failures[i].CheckIndex < failures[j].CheckIndex
		})
		reason := "CheckRunFailed"
		for _, failure := range failures {
			if failure.Phase == cctypes.InstrumentationCheckStatusConfigure {
				reason = "CheckConfigurationFailed"
				break
			}
		}
		messages := make([]string, 0, min(3, len(failures)))
		for _, failure := range failures[:min(3, len(failures))] {
			location := fmt.Sprintf("spec.config.checks[%d].instances[%d]", failure.CheckIndex, failure.InstanceIndex)
			if failure.NodeName != "" {
				location += " on " + failure.NodeName
			}
			errorMessage := failure.Error
			if errorMessage == "" {
				errorMessage = "check failed without an error message"
			}
			messages = append(messages, location+": "+errorMessage)
		}
		if len(failures) > len(messages) {
			messages = append(messages, fmt.Sprintf("and %d more failure(s)", len(failures)-len(messages)))
		}
		return instrumentation.HandlerStatus{Type: checksReadyConditionType, Status: metav1.ConditionFalse, Reason: reason, Message: strings.Join(messages, "; ")}
	}

	if expected > 0 && len(observed) >= expected {
		return instrumentation.HandlerStatus{Type: checksReadyConditionType, Status: metav1.ConditionTrue, Reason: "Running", Message: fmt.Sprintf("%d check instance(s) configured and running", expected)}
	}
	return instrumentation.HandlerStatus{Type: checksReadyConditionType, Status: metav1.ConditionUnknown, Reason: "AwaitingCheckStatus", Message: fmt.Sprintf("runtime status received for %d of %d check instance(s)", len(observed), expected)}
}
