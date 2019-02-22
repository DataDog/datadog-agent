package threeaggregator

import "testing"

func TestExtend(t *testing.T) {
	out, err := runSubmitMetric()

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
