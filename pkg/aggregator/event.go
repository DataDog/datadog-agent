package aggregator

// EventPriority represents the priority of an event
type EventPriority string

// Enumeration of the existing event priorities, and their values
const (
	EventPriorityNormal EventPriority = "normal"
	EventPriorityLow    EventPriority = "low"
)

// EventAlertType represents the alert type of an event
type EventAlertType string

// Enumeration of the existing event alert types, and their values
const (
	EventAlertTypeError   EventAlertType = "error"
	EventAlertTypeWarning EventAlertType = "warning"
	EventAlertTypeInfo    EventAlertType = "info"
	EventAlertTypeSuccess EventAlertType = "success"
)

// Event holds an event (w/ serialization to DD agent 5 intake format)
type Event struct {
	Title          string         `json:"msg_title"`
	Text           string         `json:"msg_text"`
	Ts             int64          `json:"timestamp"`
	Priority       EventPriority  `json:"priority,omitempty"`
	Host           string         `json:"host"`
	Tags           []string       `json:"tags,omitempty"`
	AlertType      EventAlertType `json:"alert_type,omitempty"`
	AggregationKey string         `json:"aggregation_key,omitempty"`
	SourceTypeName string         `json:"source_type_name,omitempty"`
}
