package loader

import "testing"

func Test_GetCheckConfig(t *testing.T) {
	config, err := getCheckConfig("foo", "tests/wrong.yaml")
	if err == nil {
		t.Fatal("Expecting error")
	}

	config, err = getCheckConfig("foo", "foo.yaml")
	if err == nil {
		t.Fatal("Expecting error")
	}

	config, err = getCheckConfig("foo", "tests/testcheck.yaml")
	if err != nil {
		t.Fatalf("Expecting nil, found: %s", err)
	}
	if config.Name != "foo" {
		t.Fatalf("Expecting `foo`, found: %s", config.Name)
	}
}

func TestNewYamlConfigProvider(t *testing.T) {
	paths := []string{"foo", "bar", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	if len(provider.paths) != len(paths) {
		t.Fatalf("Expecting length %d, found: %d", len(provider.paths), len(paths))
	}

	for i, p := range provider.paths {
		if p != paths[i] {
			t.Fatalf("Expecting %s, found: %s", paths[i], p)
		}
	}
}

func TestCollect(t *testing.T) {
	paths := []string{"tests", "foo/bar"}
	provider := NewFileConfigProvider(paths)
	configs, err := provider.Collect()

	if err != nil {
		t.Fatalf("Expecting nil, found: %s", err)
	}

	if len(configs) != 1 {
		t.Fatalf("Expecting length 1, found: %d", len(configs))
	}

	config := configs[0]
	if config.Name != "testcheck" {
		t.Fatalf("Expecting testcheck, found: %s", config.Name)
	}
}
