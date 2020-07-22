// +build linux_bpf

package probe

import "sync/atomic"

// EventsStats holds statistics about the number of lost and received events
type EventsStats struct {
	Lost         int64
	Received     int64
	PerEventType [maxEventType]int64
}

// GetLost returns the number of lost events
func (e *EventsStats) GetLost() int64 {
	return atomic.LoadInt64(&e.Lost)
}

// GetReceived returns the number of received events
func (e *EventsStats) GetReceived() int64 {
	return atomic.LoadInt64(&e.Received)
}

// GetAndResetLost returns the number of lost events and resets the counter
func (e *EventsStats) GetAndResetLost() int64 {
	return atomic.SwapInt64(&e.Lost, 0)
}

// GetAndResetReceived returns the number of received events and resets the counter
func (e *EventsStats) GetAndResetReceived() int64 {
	return atomic.SwapInt64(&e.Received, 0)
}

// GetEventCount returns the number of received events of the specified type
func (e *EventsStats) GetEventCount(eventType EventType) int64 {
	return atomic.LoadInt64(&e.PerEventType[eventType])
}

// GetAndResetEventCount returns the number of received events of the specified type and resets the counter
func (e *EventsStats) GetAndResetEventCount(eventType EventType) int64 {
	return atomic.SwapInt64(&e.PerEventType[eventType], 0)
}

// CountLost adds `count` to the counter of lost events
func (e *EventsStats) CountLost(count int64) {
	atomic.AddInt64(&e.Lost, count)
}

// CountReceived adds `count` to the counter of received events
func (e *EventsStats) CountReceived(count int64) {
	atomic.AddInt64(&e.Received, count)
}

// CountEventType adds `count` to the counter of received events of the specified type
func (e *EventsStats) CountEventType(eventType EventType, count int64) {
	atomic.AddInt64(&e.PerEventType[eventType], count)
}
