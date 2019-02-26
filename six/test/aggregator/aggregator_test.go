package testaggregator

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	ret := m.Run()

	tearDown()
	os.Exit(ret)
}

func TestSubmitMetric(t *testing.T) {
	out, err := run(`aggregator.submit_metric(None, 'id', aggregator.GAUGE, 'name', -99.0, ['foo', 'bar'], 'myhost')`)

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

func TestSubmitServiceCheck(t *testing.T) {
	out, err := run(`aggregator.submit_service_check(None, 'id', 'my.service.check', 1, ['foo', 'bar'], 'myhost', 'A message!')`)

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
