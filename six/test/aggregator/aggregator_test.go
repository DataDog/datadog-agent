package testaggregator

import (
	"fmt"
	"os"
	"regexp"
	"testing"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestSubmitMetric(t *testing.T) {
	out, err := run(`aggregator.submit_metric(None, 'id', aggregator.GAUGE, 'name', -99.0, ['foo', 21, 'bar', ["hey"]], 'myhost')`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
	if checkID != "id" {
		t.Fatalf("Unexpected id value: %s", checkID)
	}
	if metricType != 0 {
		t.Fatalf("Unexpected metricType value: %d", metricType)
	}
	if name != "name" {
		t.Fatalf("Unexpected name value: %s", name)
	}
	if value != -99.0 {
		t.Fatalf("Unexpected value: %f", value)
	}
	if hostname != "myhost" {
		t.Fatalf("Unexpected hostname value: %s", hostname)
	}
	if len(tags) != 2 {
		t.Fatalf("Unexpected tags length: %d", len(tags))
	}
	if tags[0] != "foo" || tags[1] != "bar" {
		t.Fatalf("Unexpected tags: %v", tags)
	}
}

func TestSubmitMetricParsingError(t *testing.T) {
	out, err := run(`
	aggregator.submit_metric(None, 21, aggregator.GAUGE, 'name', -99.0, ['foo', 21, 'bar', ["hey"]], 'myhost')
	`)

	if err != nil {
		t.Fatal(err)
	}
	if matched, err := regexp.Match("TypeError: argument 2 must be (str|string), not int", []byte(out)); err != nil && !matched {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestSubmitMetricTagsError(t *testing.T) {
	out, err := run(`
	aggregator.submit_metric(None, 'id', aggregator.GAUGE, 'name', -99.0, 123, 'myhost')
	`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: tags must be a sequence" {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestSubmitServiceCheck(t *testing.T) {
	out, err := run(`aggregator.submit_service_check(None, 'id', 'my.service.check', 1, ['foo', 21, 'bar', ["hey"]], 'myhost', 'A message!')`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
	if checkID != "id" {
		t.Fatalf("Unexpected id value: %s", checkID)
	}
	if scLevel != 1 {
		t.Fatalf("Unexpected metricType value: %d", scLevel)
	}
	if scName != "my.service.check" {
		t.Fatalf("Unexpected name value: %s", scName)
	}
	if hostname != "myhost" {
		t.Fatalf("Unexpected hostname value: %s", hostname)
	}
	if len(tags) != 2 {
		t.Fatalf("Unexpected tags length: %d", len(tags))
	}
	if tags[0] != "foo" || tags[1] != "bar" {
		t.Fatalf("Unexpected tags: %v", tags)
	}
	if scMessage != "A message!" {
		t.Fatalf("Unexpected name value: %s", scMessage)
	}
}

func TestSubmitServiceCheckParsingError(t *testing.T) {
	out, err := run(`aggregator.submit_service_check(None, 123, 'my.service.check', 1, ['foo', 21, 'bar', ["hey"]], 'myhost', 'A message!')`)

	if err != nil {
		t.Fatal(err)
	}
	if matched, err := regexp.Match("TypeError: argument 1 must be (str|string), not int", []byte(out)); err != nil && !matched {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestSubmitServiceCheckTagsError(t *testing.T) {
	out, err := run(`aggregator.submit_service_check(None, 'id', 'my.service.check', 1, 123, 'myhost', 'A message!')`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: tags must be a sequence" {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestSubmitEvent(t *testing.T) {
	code := `
	ev = {
		'timestamp': 123456,
		'event_type': 'my.event',
		'host': 'myhost',
		'msg_text': 'Event message',
		'msg_title': 'Event title',
		'alert_type': 'foo',
		'source_type_name': 'test',
		'event_object': 'myhost',
		'tags': ['foo', 21, 'bar', ["hey"]],
		'priority': 'high',
		'aggregation_key': 'aggregate',
	}
	aggregator.submit_event(None, 'submit_event_id', ev)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
	if checkID != "submit_event_id" {
		t.Fatalf("Unexpected id value: %s", checkID)
	}
	if _event.title != "Event title" {
		t.Fatalf("Unexpected event title: %s", _event.title)
	}
	if _event.text != "Event message" {
		t.Fatalf("Unexpected event text: %s", _event.text)
	}
	if _event.ts != 123456 {
		t.Fatalf("Unexpected event ts: %d", _event.ts)
	}
	if _event.priority != "high" {
		t.Fatalf("Unexpected event priority: %s", _event.priority)
	}
	if _event.host != "myhost" {
		t.Fatalf("Unexpected event host: %s", _event.host)
	}
	if _event.alertType != "foo" {
		t.Fatalf("Unexpected event alert_type: %s", _event.alertType)
	}
	if _event.aggregationKey != "aggregate" {
		t.Fatalf("Unexpected event aggregation_key: %s", _event.aggregationKey)
	}
	if _event.sourceTypeName != "test" {
		t.Fatalf("Unexpected event source_type_name: %s", _event.sourceTypeName)
	}
	if _event.eventType != "my.event" {
		t.Fatalf("Unexpected event event_type: %s", _event.eventType)
	}
	if len(_event.tags) != 2 {
		t.Fatalf("Unexpected tags length: %d", len(_event.tags))
	}
	if _event.tags[0] != "foo" || _event.tags[1] != "bar" {
		t.Fatalf("Unexpected tags: %v", _event.tags)
	}
}

func TestSubmitEventMissingFields(t *testing.T) {
	code := `
	ev = {
		'msg_text': 'Event message',
	}
	aggregator.submit_event(None, 'submit_event_id', ev)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
	if checkID != "submit_event_id" {
		t.Fatalf("Unexpected id value: %s", checkID)
	}
	if _event.title != "" {
		t.Fatalf("Unexpected event title: %s", _event.title)
	}
	if _event.text != "Event message" {
		t.Fatalf("Unexpected event text: %s", _event.text)
	}
	if _event.ts != 0 {
		t.Fatalf("Unexpected event ts: %d", _event.ts)
	}
	if _event.priority != "" {
		t.Fatalf("Unexpected event priority: %s", _event.priority)
	}
	if _event.host != "" {
		t.Fatalf("Unexpected event host: %s", _event.host)
	}
	if _event.alertType != "" {
		t.Fatalf("Unexpected event alert_type: %s", _event.alertType)
	}
	if _event.aggregationKey != "" {
		t.Fatalf("Unexpected event aggregation_key: %s", _event.aggregationKey)
	}
	if _event.sourceTypeName != "" {
		t.Fatalf("Unexpected event source_type_name: %s", _event.sourceTypeName)
	}
	if _event.eventType != "" {
		t.Fatalf("Unexpected event event_type: %s", _event.eventType)
	}
	if _event.tags != nil {
		t.Fatal("Tags should be nil")
	}
}

func TestEventCheckEventNotDict(t *testing.T) {
	code := `
	aggregator.submit_event(None, 'id', "I should be a dict")
	`
	out, err := run(code)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: event must be a dict" {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestEventCheckParsingError(t *testing.T) {
	code := `
	aggregator.submit_event(None, 21, {})
	`
	out, err := run(code)

	if err != nil {
		t.Fatal(err)
	}
	if matched, err := regexp.Match("TypeError: argument 2 must be (str|string), not int", []byte(out)); err != nil && !matched {
		t.Errorf("wrong printed value: '%s'", out)
	}
}

func TestEventCheckTagsError(t *testing.T) {
	code := `
	ev = {
		'timestamp': 123456,
		'event_type': 'my.event',
		'host': 'myhost',
		'msg_text': 'Event message',
		'msg_title': 'Event title',
		'alert_type': 'foo',
		'source_type_name': 'test',
		'event_object': 'myhost',
		'tags': 123,
		'priority': 'high',
		'aggregation_key': 'aggregate',
	}
	aggregator.submit_event(None, 'submit_event_id', ev)
	`
	out, err := run(code)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: tags must be a sequence" {
		t.Errorf("wrong printed value: '%s'", out)
	}
}
