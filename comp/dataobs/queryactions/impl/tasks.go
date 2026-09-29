// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"go.yaml.in/yaml/v3"
)

// One-off tasks share the DO_QUERY_ACTIONS product with monitor configs but never touch them.
// Each task becomes its own run_once check config, derived from the connection fields of the
// user's DO-enabled instance. Monitor configs replace the user's instance in place; tasks never do,
// so the user's check and the monitor checks are never restarted because of a task.
//
// Lifecycle: a task is scheduled once, when its config first appears in an RC snapshot. It is left
// alone while it stays in the snapshot and unscheduled when the backend removes its config (the task
// finished, was cancelled or expired). A task the agent cannot run fails immediately: the agent sends
// a task-level do-query-results error event and reports RC apply state ERROR.

// taskConfigIDPattern matches the RC config IDs of one-off tasks: do-<platform>-once-<task_id>.
var taskConfigIDPattern = regexp.MustCompile(`^do-[a-z]+-once-`)

const (
	// taskProviderName is the autodiscovery provider that delivers task check configs. It is separate
	// from the monitor provider so task schedules never go through the monitor channel, which keeps
	// only the latest snapshot.
	taskProviderName = "do-query-actions-tasks"
	// taskCheckSource is the Source of every task check config, shown by `agent configcheck`.
	taskCheckSource = "do-query-actions:task"
	// doQueryResultsEventType is the event platform track the database checks send query results
	// on (EVENT_TRACK_TYPE in the Python checks).
	doQueryResultsEventType = "do-query-results"

	taskKind = "task"

	taskErrorKindNoMatchingInstance = "no_matching_instance"
	taskErrorKindExpired            = "expired"
	taskErrorPhaseSchedule          = "schedule"
)

// taskConnectionFields are the instance fields a task check copies from the matched user instance:
// how to reach and authenticate to the database, and how the check identifies it. Everything else
// (dbm, data_observability, custom_queries, collection settings...) is left out, so a task check
// only ever runs its own statements. Secret handles (ENC[...]) are copied as-is and decrypted by
// autodiscovery when the task config is scheduled.
var taskConnectionFields = []string{
	"host", "server", "port", "sock",
	"username", "user", "password", "pass",
	"defaults_file", "connect_timeout", "read_timeout", "charset", "ssl",
	"aws", "azure", "gcp",
	"reported_hostname", "database_identifier", "exclude_hostname", "empty_default_hostname",
	"disable_generic_tags", "enable_legacy_tags_normalization", "tags", "service",
}

var errTaskUnsupportedPlatform = errors.New("one-off tasks are only supported for mysql")

// trackedTask is a task config seen in an RC snapshot. checkConfig is nil when the task failed
// before a check was scheduled.
type trackedTask struct {
	status      state.ApplyStatus
	checkConfig *integration.Config
}

// taskConfigUpdate is a task config from an RC snapshot, keyed by its config ID.
type taskConfigUpdate struct {
	path string
	raw  state.RawConfig
}

// isTaskConfigID reports whether an RC config ID belongs to a one-off task.
func isTaskConfigID(configID string) bool {
	return taskConfigIDPattern.MatchString(configID)
}

// splitTaskUpdates separates one-off task configs from monitor configs so that each path only ever
// sees its own configs. Task configs are routed by RC config ID; the payload's config_id is only
// consulted when the RC metadata carries no ID.
func splitTaskUpdates(updates map[string]state.RawConfig) (map[string]state.RawConfig, map[string]taskConfigUpdate) {
	monitors := make(map[string]state.RawConfig, len(updates))
	tasks := make(map[string]taskConfigUpdate)
	for path, raw := range updates {
		configID := raw.Metadata.ID
		if configID == "" {
			var header struct {
				ConfigID string `json:"config_id"`
			}
			// A payload that doesn't parse stays on the monitor path, which reports the error.
			_ = json.Unmarshal(raw.Config, &header)
			configID = header.ConfigID
		}
		if isTaskConfigID(configID) {
			tasks[configID] = taskConfigUpdate{path: path, raw: raw}
			continue
		}
		monitors[path] = raw
	}
	return monitors, tasks
}

// onTaskUpdate reconciles the task configs of an RC snapshot with the tasks already seen, and
// returns the check configs to schedule (new tasks) and unschedule (tasks whose config is gone).
// A task already seen is never rescheduled: its check may have run already, and a second schedule
// would run its statements again.
func (c *component) onTaskUpdate(updates map[string]taskConfigUpdate, applyStatus func(string, state.ApplyStatus)) integration.ConfigChanges {
	changes := integration.ConfigChanges{}

	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	configIDs := make([]string, 0, len(updates))
	for configID := range updates {
		configIDs = append(configIDs, configID)
	}
	sort.Strings(configIDs)

	for _, configID := range configIDs {
		update := updates[configID]
		if tracked, ok := c.tasks[configID]; ok {
			// Re-report the outcome in case RC reset the config's apply state.
			applyStatus(update.path, tracked.status)
			continue
		}
		tracked := c.startTask(configID, update.raw)
		c.tasks[configID] = tracked
		applyStatus(update.path, tracked.status)
		if tracked.checkConfig != nil {
			changes.Schedule = append(changes.Schedule, *tracked.checkConfig)
		}
	}

	for configID, tracked := range c.tasks {
		if _, ok := updates[configID]; ok {
			continue
		}
		delete(c.tasks, configID)
		if tracked.checkConfig != nil {
			changes.Unschedule = append(changes.Unschedule, *tracked.checkConfig)
			c.log.Infof("Task config %s absent from RC snapshot, unscheduling its check", configID)
		}
	}

	return changes
}

// startTask validates a new task and, when the agent can run it, builds its check config. Every
// failure is final: the backend doesn't resend a task, so it either fails the task from the error
// event or lets it expire.
//
// Failure handling:
//   - malformed payload: apply state ERROR only. The RC schema rejects such payloads at write
//     time, and without a valid task the event could not be correlated anyway; the task expires.
//   - expires_at passed (e.g. the agent restarted after the backend gave up): "expired" event.
//   - no local DO-enabled instance matches db_identifier, or the platform has no task mode:
//     "no_matching_instance" event. The backend fails the task instead of retrying, because its
//     targeting would pick this agent again.
func (c *component) startTask(configID string, raw state.RawConfig) *trackedTask {
	var payload DOTaskPayload
	if err := json.Unmarshal(raw.Config, &payload); err != nil {
		c.log.Warnf("Failed to unmarshal DO_QUERY_ACTIONS task config %s: %v", configID, err)
		return failedTask(err)
	}
	if err := validateTaskPayload(configID, &payload); err != nil {
		c.log.Warnf("Invalid DO_QUERY_ACTIONS task config %s: %v", configID, err)
		return failedTask(err)
	}

	integrationName, _ := integrationForConfigID(configID)
	if !c.now().Before(time.Unix(payload.Task.ExpiresAt, 0)) {
		err := fmt.Errorf("task %s expired at %s before the agent could start it", payload.Task.TaskID, time.Unix(payload.Task.ExpiresAt, 0).UTC().Format(time.RFC3339))
		c.log.Warnf("Not running task config %s: %v", configID, err)
		c.sendTaskError(configID, &payload, integrationName, taskErrorKindExpired, err)
		return failedTask(err)
	}

	if integrationName != "mysql" {
		err := fmt.Errorf("%w, task config %s targets %s", errTaskUnsupportedPlatform, configID, integrationName)
		c.log.Warnf("Not running task config %s: %v", configID, err)
		c.sendTaskError(configID, &payload, integrationName, taskErrorKindNoMatchingInstance, err)
		return failedTask(err)
	}

	baseCfg, instance, err := c.resolveTaskInstance(configID, &payload.DBIdentifier)
	if err != nil {
		c.log.Warnf("No matching integration instance for task config %s: %v", configID, err)
		c.sendTaskError(configID, &payload, integrationName, taskErrorKindNoMatchingInstance, err)
		return failedTask(err)
	}

	checkConfig, err := buildTaskCheckConfig(configID, &payload, baseCfg, instance)
	if err != nil {
		c.log.Errorf("Failed to build check config for task config %s: %v", configID, err)
		return failedTask(err)
	}

	c.log.Infof("Scheduling one-off Data Observability task %s (%d statements)", configID, len(payload.Task.Statements))
	return &trackedTask{
		status:      state.ApplyStatus{State: state.ApplyStateAcknowledged},
		checkConfig: &checkConfig,
	}
}

func failedTask(err error) *trackedTask {
	return &trackedTask{status: state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}}
}

// validateTaskPayload checks the task-level structure. Statements are validated by the
// integration, so that an invalid statement fails alone instead of failing the whole task.
func validateTaskPayload(configID string, payload *DOTaskPayload) error {
	if payload.Kind != taskKind {
		return fmt.Errorf("unexpected kind %q, want %q", payload.Kind, taskKind)
	}
	if payload.ConfigID != "" && payload.ConfigID != configID {
		return fmt.Errorf("payload config_id %q does not match RC config ID %q", payload.ConfigID, configID)
	}
	if payload.Task.TaskID == "" {
		return errors.New("empty task_id")
	}
	// The config ID embeds the task ID, so one task can never become two checks.
	if taskConfigIDPattern.ReplaceAllString(configID, "") != payload.Task.TaskID {
		return fmt.Errorf("RC config ID %q does not end with task_id %q", configID, payload.Task.TaskID)
	}
	if payload.Task.ExpiresAt <= 0 {
		return errors.New("missing expires_at")
	}
	if len(payload.Task.Statements) == 0 {
		return errors.New("task has no statements")
	}
	return nil
}

// resolveTaskInstance finds the local DO-enabled instance a task targets, using the same identifier
// matching as monitor configs.
//
// When a monitor config already targets the instance, autodiscovery reports the monitor's copy of
// it instead of the user's config, and that copy has no init_config. The original config stored for
// the monitor is therefore tried first; otherwise the configs autodiscovery currently reports are
// searched. Task configs themselves never match: they have no data_observability section.
func (c *component) resolveTaskInstance(configID string, dbID *DBIdentifier) (*integration.Config, map[string]any, error) {
	if dbID.Host == "" {
		return nil, nil, errEmptyIdentifierHost
	}
	expectedIntegration, _ := integrationForConfigID(configID)

	c.activeConfigsMu.Lock()
	monitorIDs := make([]string, 0, len(c.activeConfigs))
	for monitorID := range c.activeConfigs {
		monitorIDs = append(monitorIDs, monitorID)
	}
	sort.Strings(monitorIDs)
	bases := make([]*integration.Config, 0, len(monitorIDs))
	for _, monitorID := range monitorIDs {
		bases = append(bases, c.activeConfigs[monitorID].baseCfg)
	}
	c.activeConfigsMu.Unlock()

	for _, base := range bases {
		if base.Name != expectedIntegration {
			continue
		}
		if instance, _, _ := c.findMatchingInstance(base, dbID, nil); instance != nil {
			return base, instance, nil
		}
	}

	cfg, instance, _, err := c.findMatchingConfig(configID, dbID, nil)
	return cfg, instance, err
}

// buildTaskCheckConfig builds the run_once check config of a task. The instance holds only the
// connection fields of the matched instance plus the do_task block, which the integration runs
// instead of its regular collection. The task ID inside do_task makes the digest unique per task.
func buildTaskCheckConfig(configID string, payload *DOTaskPayload, baseCfg *integration.Config, instance map[string]any) (integration.Config, error) {
	fields := make(map[string]any, len(taskConnectionFields)+2)
	for _, key := range taskConnectionFields {
		if value, ok := instance[key]; ok {
			fields[key] = value
		}
	}

	statements := make([]map[string]any, 0, len(payload.Task.Statements))
	for _, statement := range payload.Task.Statements {
		statements = append(statements, map[string]any{
			"id":     statement.ID,
			"dbname": statement.DBName,
			// Double-quoted for the same reason as monitor queries: see buildCheckConfig.
			"query":           &yaml.Node{Kind: yaml.ScalarNode, Style: yaml.DoubleQuotedStyle, Value: statement.Query},
			"timeout_seconds": statement.TimeoutSeconds,
			"max_rows":        statement.MaxRows,
		})
	}

	// Python checks need run_once for a one-shot run; min_collection_interval 0 means the default 15s.
	fields["run_once"] = true
	fields["do_task"] = map[string]any{
		"config_id":  configID,
		"task_id":    payload.Task.TaskID,
		"expires_at": payload.Task.ExpiresAt,
		"statements": statements,
	}

	instanceYAML, err := yaml.Marshal(fields)
	if err != nil {
		return integration.Config{}, fmt.Errorf("failed to marshal task check instance: %w", err)
	}

	return integration.Config{
		Name:       baseCfg.Name,
		Source:     taskCheckSource,
		Provider:   taskProviderName,
		NodeName:   baseCfg.NodeName,
		InitConfig: baseCfg.InitConfig,
		Instances:  []integration.Data{instanceYAML},
	}, nil
}

// taskErrorEvent is the do-query-results event of a task the agent could not start. It has the
// shape of a statement error result, with a task ID and no statement ID.
type taskErrorEvent struct {
	Timestamp  int64      `json:"timestamp"`
	ConfigID   string     `json:"config_id"`
	TaskID     string     `json:"task_id"`
	ChunkIndex int        `json:"chunk_index"`
	ChunkCount int        `json:"chunk_count"`
	DBType     string     `json:"db_type"`
	DBHost     string     `json:"db_host"`
	Status     string     `json:"status"`
	Columns    []string   `json:"columns"`
	Rows       [][]string `json:"rows"`
	RowCount   int        `json:"row_count"`
	DurationS  float64    `json:"duration_s"`
	Error      string     `json:"error"`
	ErrorKind  string     `json:"error_kind"`
	ErrorPhase string     `json:"error_phase"`
}

// sendTaskError sends a task-level error result so the backend fails the task in seconds instead of
// waiting for it to expire. Sending is best effort: when the event platform forwarder is unavailable
// or its queue is full the event is dropped, the RC apply state still records the error, and the
// backend marks the task expired at expires_at.
func (c *component) sendTaskError(configID string, payload *DOTaskPayload, dbType, errorKind string, taskErr error) {
	body, err := json.Marshal(taskErrorEvent{
		Timestamp:  c.now().UnixMilli(),
		ConfigID:   configID,
		TaskID:     payload.Task.TaskID,
		ChunkIndex: 0,
		ChunkCount: 1,
		DBType:     dbType,
		DBHost:     payload.DBIdentifier.Host,
		Status:     "error",
		Columns:    []string{},
		Rows:       [][]string{},
		Error:      taskErr.Error(),
		ErrorKind:  errorKind,
		ErrorPhase: taskErrorPhaseSchedule,
	})
	if err != nil {
		c.log.Errorf("Failed to marshal task error event for %s: %v", configID, err)
		return
	}

	if c.eventPlatform == nil {
		c.log.Warnf("Event platform forwarder unavailable, dropping task error event for %s", configID)
		return
	}
	forwarder, ok := c.eventPlatform.Get()
	if !ok || forwarder == nil {
		c.log.Warnf("Event platform forwarder unavailable, dropping task error event for %s", configID)
		return
	}
	msg := message.NewMessage(body, nil, "", c.now().UnixNano())
	if err := forwarder.SendEventPlatformEvent(msg, doQueryResultsEventType); err != nil {
		c.log.Warnf("Failed to send task error event for %s: %v", configID, err)
	}
}

// taskChangesQueue holds task config changes until autodiscovery consumes them. Unlike the monitor
// channel, it never drops or merges changes: a lost schedule would lose the task, and merging a
// schedule with a later unschedule of the same check could apply them in the wrong order.
type taskChangesQueue struct {
	mu      sync.Mutex
	pending []integration.ConfigChanges
	// notify wakes the stream goroutine; capacity 1 because one wake-up drains the whole queue.
	notify chan struct{}
}

func newTaskChangesQueue() *taskChangesQueue {
	return &taskChangesQueue{notify: make(chan struct{}, 1)}
}

func (q *taskChangesQueue) push(changes integration.ConfigChanges) {
	if changes.IsEmpty() {
		return
	}
	q.mu.Lock()
	q.pending = append(q.pending, changes)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *taskChangesQueue) pop() (integration.ConfigChanges, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return integration.ConfigChanges{}, false
	}
	changes := q.pending[0]
	q.pending = q.pending[1:]
	return changes, true
}

// unpop puts back changes that were popped but not delivered, ahead of everything else.
func (q *taskChangesQueue) unpop(changes integration.ConfigChanges) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append([]integration.ConfigChanges{changes}, q.pending...)
}

// taskProvider is the autodiscovery streaming provider for task check configs.
type taskProvider struct {
	queue *taskChangesQueue
}

// String returns the name of the provider
func (p *taskProvider) String() string {
	return taskProviderName
}

// GetConfigErrors is required by the ConfigProvider interface. Task failures are reported through
// RC apply states and task error events.
func (p *taskProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream delivers queued task changes in order. Like the monitor provider, it sends an empty
// ConfigChanges first to unblock autodiscovery's LoadAndRun. The channel is unbuffered, so a change
// counts as delivered only once autodiscovery has received it; a change still undelivered when ctx
// ends goes back to the front of the queue for the next Stream call.
func (p *taskProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges)

	go func() {
		defer close(outCh)
		select {
		case outCh <- integration.ConfigChanges{}:
		case <-ctx.Done():
			return
		}
		for {
			changes, ok := p.queue.pop()
			if !ok {
				select {
				case <-p.queue.notify:
					continue
				case <-ctx.Done():
					return
				}
			}
			select {
			case outCh <- changes:
			case <-ctx.Done():
				p.queue.unpop(changes)
				return
			}
		}
	}()

	return outCh
}
