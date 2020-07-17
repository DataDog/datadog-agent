package probe

import "sync/atomic"

type EventsStats struct {
	Lost         int64
	Received     int64
	PerEventType [maxEventType]int64
}

func (e *EventsStats) GetLost() int64 {
	return atomic.LoadInt64(&e.Lost)
}

func (e *EventsStats) GetReceived() int64 {
	return atomic.LoadInt64(&e.Received)
}

func (e *EventsStats) GetAndResetLost() int64 {
	return atomic.SwapInt64(&e.Lost, 0)
}

func (e *EventsStats) GetAndResetReceived() int64 {
	return atomic.SwapInt64(&e.Received, 0)
}

func (e *EventsStats) GetEventCount(eventType ProbeEventType) int64 {
	return atomic.LoadInt64(&e.PerEventType[eventType])
}

func (e *EventsStats) GetAndResetEventCount(eventType ProbeEventType) int64 {
	return atomic.SwapInt64(&e.PerEventType[eventType], 0)
}

func (e *EventsStats) CountLost(count int64) {
	atomic.AddInt64(&e.Lost, count)
}

func (e *EventsStats) CountReceived(count int64) {
	atomic.AddInt64(&e.Received, count)
}

func (e *EventsStats) CountEventType(eventType ProbeEventType, count int64) {
	atomic.AddInt64(&e.PerEventType[eventType], count)
}
