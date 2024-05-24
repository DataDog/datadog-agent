// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package serializers

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// ContainerContextSerializer serializes a container context to JSON
// easyjson:json
type ContainerContextSerializer struct {
	// Container ID
	ID string `json:"id,omitempty"`
	// Creation time of the container
	CreatedAt *utils.EasyjsonTime `json:"created_at,omitempty"`
	// Variables values
	Variables Variables `json:"variables,omitempty"`
}

// Variables serializes the variable values
// easyjson:json
type Variables map[string]interface{}

// MatchedRuleSerializer serializes a rule
// easyjson:json
type MatchedRuleSerializer struct {
	// ID of the rule
	ID string `json:"id,omitempty"`
	// Version of the rule
	Version string `json:"version,omitempty"`
	// Tags of the rule
	Tags []string `json:"tags,omitempty"`
	// Name of the policy that introduced the rule
	PolicyName string `json:"policy_name,omitempty"`
	// Version of the policy that introduced the rule
	PolicyVersion string `json:"policy_version,omitempty"`
}

// EventContextSerializer serializes an event context to JSON
// easyjson:json
type EventContextSerializer struct {
	// Event name
	Name string `json:"name,omitempty"`
	// Event category
	Category string `json:"category,omitempty"`
	// Event outcome
	Outcome string `json:"outcome,omitempty"`
	// True if the event was asynchronous
	Async bool `json:"async,omitempty"`
	// The list of rules that the event matched (only valid in the context of an anomaly)
	MatchedRules []MatchedRuleSerializer `json:"matched_rules,omitempty"`
	// Variables values
	Variables Variables `json:"variables,omitempty"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessSerializer
	// Parent process
	Parent *ProcessSerializer `json:"parent,omitempty"`
	// Ancestor processes
	Ancestors []*ProcessSerializer `json:"ancestors,omitempty"`
	// Variables values
	Variables Variables `json:"variables,omitempty"`
	// True if the ancestors list was truncated because it was too big
	TruncatedAncestors bool `json:"truncated_ancestors,omitempty"`
}

// IPPortSerializer is used to serialize an IP and Port context to JSON
// easyjson:json
type IPPortSerializer struct {
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// IPPortFamilySerializer is used to serialize an IP, port, and address family context to JSON
// easyjson:json
type IPPortFamilySerializer struct {
	// Address family
	Family string `json:"family"`
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// NetworkContextSerializer serializes the network context to JSON
// easyjson:json
type NetworkContextSerializer struct {
	// device is the network device on which the event was captured
	Device *NetworkDeviceSerializer `json:"device,omitempty"`

	// l3_protocol is the layer 3 protocol name
	L3Protocol string `json:"l3_protocol"`
	// l4_protocol is the layer 4 protocol name
	L4Protocol string `json:"l4_protocol"`
	// source is the emitter of the network event
	Source IPPortSerializer `json:"source"`
	// destination is the receiver of the network event
	Destination IPPortSerializer `json:"destination"`
	// size is the size in bytes of the network event
	Size uint32 `json:"size"`
}

// AWSSecurityCredentialsSerializer serializes the security credentials from an AWS IMDS request
// easyjson:json
type AWSSecurityCredentialsSerializer struct {
	// code is the IMDS server code response
	Code string `json:"code"`
	// type is the security credentials type
	Type string `json:"type"`
	// access_key_id is the unique access key ID of the credentials
	AccessKeyID string `json:"access_key_id"`
	// last_updated is the last time the credentials were updated
	LastUpdated string `json:"last_updated"`
	// expiration is the expiration date of the credentials
	Expiration string `json:"expiration"`
}

// AWSIMDSEventSerializer serializes an AWS IMDS event to JSON
// easyjson:json
type AWSIMDSEventSerializer struct {
	// is_imds_v2 reports if the IMDS event follows IMDSv1 or IMDSv2 conventions
	IsIMDSv2 bool `json:"is_imds_v2"`
	// SecurityCredentials holds the scrubbed data collected on the security credentials
	SecurityCredentials *AWSSecurityCredentialsSerializer `json:"security_credentials,omitempty"`
}

// IMDSEventSerializer serializes an IMDS event to JSON
// easyjson:json
type IMDSEventSerializer struct {
	// type is the type of IMDS event
	Type string `json:"type"`
	// cloud_provider is the intended cloud provider of the IMDS event
	CloudProvider string `json:"cloud_provider"`
	// url is the url of the IMDS request
	URL string `json:"url,omitempty"`
	// host is the host of the HTTP protocol
	Host string `json:"host,omitempty"`
	// user_agent is the user agent of the HTTP client
	UserAgent string `json:"user_agent,omitempty"`
	// server is the server header of a response
	Server string `json:"server,omitempty"`

	// AWS holds the AWS specific data parsed from the IMDS event
	AWS *AWSIMDSEventSerializer `json:"aws,omitempty"`
}

// DNSQuestionSerializer serializes a DNS question to JSON
// easyjson:json
type DNSQuestionSerializer struct {
	// class is the class looked up by the DNS question
	Class string `json:"class"`
	// type is a two octet code which specifies the DNS question type
	Type string `json:"type"`
	// name is the queried domain name
	Name string `json:"name"`
	// size is the total DNS request size in bytes
	Size uint16 `json:"size"`
	// count is the total count of questions in the DNS request
	Count uint16 `json:"count"`
}

// DNSEventSerializer serializes a DNS event to JSON
// easyjson:json
type DNSEventSerializer struct {
	// id is the unique identifier of the DNS request
	ID uint16 `json:"id"`
	// question is a DNS question for the DNS request
	Question DNSQuestionSerializer `json:"question"`
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	// Span ID used for APM correlation
	SpanID uint64 `json:"span_id,omitempty"`
	// Trace ID used for APM correlation
	TraceID uint64 `json:"trace_id,omitempty"`
}

// ExitEventSerializer serializes an exit event to JSON
// easyjson:json
type ExitEventSerializer struct {
	// Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)
	Cause string `json:"cause"`
	// Exit code of the process or number of the signal that caused the process to terminate
	Code uint32 `json:"code"`
}

// BaseEventSerializer serializes an event to JSON
// easyjson:json
type BaseEventSerializer struct {
	EventContextSerializer `json:"evt,omitempty"`
	Date                   utils.EasyjsonTime `json:"date,omitempty"`

	*FileEventSerializer        `json:"file,omitempty"`
	*ExitEventSerializer        `json:"exit,omitempty"`
	*ProcessContextSerializer   `json:"process,omitempty"`
	*ContainerContextSerializer `json:"container,omitempty"`
}

func newMatchedRulesSerializer(r *model.MatchedRule) MatchedRuleSerializer {
	mrs := MatchedRuleSerializer{
		ID:            r.RuleID,
		Version:       r.RuleVersion,
		PolicyName:    r.PolicyName,
		PolicyVersion: r.PolicyVersion,
		Tags:          make([]string, 0, len(r.RuleTags)),
	}

	for tagName, tagValue := range r.RuleTags {
		mrs.Tags = append(mrs.Tags, tagName+":"+tagValue)
	}

	return mrs
}

// nolint: deadcode, unused
func newDNSEventSerializer(d *model.DNSEvent) *DNSEventSerializer {
	return &DNSEventSerializer{
		ID: d.ID,
		Question: DNSQuestionSerializer{
			Class: model.QClass(d.Class).String(),
			Type:  model.QType(d.Type).String(),
			Name:  d.Name,
			Size:  d.Size,
			Count: d.Count,
		},
	}
}

// nolint: deadcode, unused
func newAWSSecurityCredentialsSerializer(creds *model.AWSSecurityCredentials) *AWSSecurityCredentialsSerializer {
	return &AWSSecurityCredentialsSerializer{
		Code:        creds.Code,
		Type:        creds.Type,
		LastUpdated: creds.LastUpdated,
		Expiration:  creds.ExpirationRaw,
		AccessKeyID: creds.AccessKeyID,
	}
}

// nolint: deadcode, unused
func newIMDSEventSerializer(e *model.IMDSEvent) *IMDSEventSerializer {
	var aws *AWSIMDSEventSerializer
	if e.CloudProvider == model.IMDSAWSCloudProvider {
		aws = &AWSIMDSEventSerializer{
			IsIMDSv2: e.AWS.IsIMDSv2,
		}
		if len(e.AWS.SecurityCredentials.AccessKeyID) > 0 {
			aws.SecurityCredentials = newAWSSecurityCredentialsSerializer(&e.AWS.SecurityCredentials)
		}
	}

	return &IMDSEventSerializer{
		Type:          e.Type,
		CloudProvider: e.CloudProvider,
		URL:           e.URL,
		Host:          e.Host,
		UserAgent:     e.UserAgent,
		Server:        e.Server,
		AWS:           aws,
	}
}

// nolint: deadcode, unused
func newIPPortSerializer(c *model.IPPortContext) IPPortSerializer {
	return IPPortSerializer{
		IP:   c.IPNet.IP.String(),
		Port: c.Port,
	}
}

// nolint: deadcode, unused
func newIPPortFamilySerializer(c *model.IPPortContext, family string) IPPortFamilySerializer {
	return IPPortFamilySerializer{
		IP:     c.IPNet.IP.String(),
		Port:   c.Port,
		Family: family,
	}
}

func newExitEventSerializer(e *model.Event) *ExitEventSerializer {
	return &ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

// NewBaseEventSerializer creates a new event serializer based on the event type
func NewBaseEventSerializer(event *model.Event, opts *eval.Opts) *BaseEventSerializer {
	pc := event.ProcessContext

	eventType := model.EventType(event.Type)

	s := &BaseEventSerializer{
		EventContextSerializer: EventContextSerializer{
			Name:      eventType.String(),
			Variables: newVariablesContext(event, opts, ""),
		},
		ProcessContextSerializer: newProcessContextSerializer(pc, event),
		Date:                     utils.NewEasyjsonTime(event.ResolveEventTime()),
	}
	if s.ProcessContextSerializer != nil {
		s.ProcessContextSerializer.Variables = newVariablesContext(event, opts, "process.")
	}

	if event.IsAnomalyDetectionEvent() && len(event.Rules) > 0 {
		s.EventContextSerializer.MatchedRules = make([]MatchedRuleSerializer, 0, len(event.Rules))
		for _, r := range event.Rules {
			s.EventContextSerializer.MatchedRules = append(s.EventContextSerializer.MatchedRules, newMatchedRulesSerializer(r))
		}
	}

	s.Category = model.GetEventTypeCategory(eventType.String())

	switch eventType {
	case model.ExitEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.ExitEventSerializer = newExitEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	}

	return s
}

func newVariablesContext(e *model.Event, opts *eval.Opts, prefix string) (variables Variables) {
	if opts != nil && opts.VariableStore != nil {
		store := opts.VariableStore
		for name, variable := range store.Variables {
			if _, found := model.SECLVariables[name]; found {
				continue
			}

			if (prefix != "" && !strings.HasPrefix(name, prefix)) ||
				(prefix == "" && strings.Contains(name, ".")) {
				continue
			}

			evaluator := variable.GetEvaluator()
			if evaluator, ok := evaluator.(eval.Evaluator); ok {
				value := evaluator.Eval(eval.NewContext(e))
				if variables == nil {
					variables = Variables{}
				}
				if value != nil {
					switch value := value.(type) {
					case []string:
						for _, value := range value {
							if scrubbed, err := scrubber.ScrubString(value); err == nil {
								variables[strings.TrimPrefix(name, prefix)] = scrubbed
							}
						}
					case string:
						if scrubbed, err := scrubber.ScrubString(value); err == nil {
							variables[strings.TrimPrefix(name, prefix)] = scrubbed
						}
					default:
						variables[strings.TrimPrefix(name, prefix)] = value
					}
				}
			}
		}
	}
	return variables
}
