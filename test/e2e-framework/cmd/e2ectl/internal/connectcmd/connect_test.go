// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"k8s.io/client-go/tools/clientcmd"
)

// The tests are hermetic: private store (E2ECTL_HOME), private HOME, private
// KUBECONFIG and a local runner profile (no CI variables), so nothing on the
// machine is read or written.

func testStore(t *testing.T) *envstore.Store {
	t.Helper()
	t.Setenv("E2ECTL_HOME", t.TempDir())
	store, err := envstore.New()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// addEnv hand-writes a store entry (meta + snapshot) without going through a
// driver: connect only needs the stored artifacts.
func addEnv(t *testing.T, store *envstore.Store, name, base, fakeintakeURL string, resources provisioner.RawResources) envstore.Entry {
	t.Helper()
	entry := envstore.Entry{Name: name, Dir: filepath.Join(store.EnvsDir(), name)}
	if err := os.MkdirAll(entry.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"name":%q,"base":%q,"status":"ready","fakeintake_url":%q}`, name, base, fakeintakeURL)
	if err := os.WriteFile(filepath.Join(entry.Dir, "meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	if resources != nil {
		if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, nil); err != nil {
			t.Fatal(err)
		}
	}
	return entry
}

func clusterSnapshot(kubeconfigYAML string) provisioner.RawResources {
	res, err := json.Marshal(map[string]string{"clusterName": "dev", "kubeConfig": kubeconfigYAML})
	if err != nil {
		panic(err)
	}
	return provisioner.RawResources{"kubernetesCluster": res}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The managed ssh entry is written once, replaces only its own block on
// re-runs, leaves other blocks and unrelated config alone, and appends the
// Include line exactly once.
func TestSSHHostEntryIsManagedAndIdempotent(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CI", "")
	t.Setenv("E2E_PROFILE", "")
	key := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(key, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("E2E_AWS_PRIVATE_KEY_PATH", key)

	// pre-existing state that must survive untouched: another managed block
	// and unrelated main-config content
	managedDir := filepath.Join(home, ".ssh", "config.d")
	if err := os.MkdirAll(managedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	otherBlock := "# e2ectl:other begin\nHost other\n    HostName 203.0.113.99\n# e2ectl:other end\n"
	if err := os.WriteFile(filepath.Join(managedDir, "e2ectl"), []byte(otherBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(home, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(mainConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	unrelated := "Host preexisting\n    HostName example.com\n"
	if err := os.WriteFile(mainConfig, []byte(unrelated), 0o600); err != nil {
		t.Fatal(err)
	}

	addEnv(t, store, "dev", "ec2-host", "http://127.0.0.1:30080",
		provisioner.RawResources{"remoteHost": []byte(`{"cloudProvider":"aws","address":"203.0.113.10","port":22,"username":"ubuntu"}`)})
	if err := Run([]string{"dev"}, store); err != nil {
		t.Fatal(err)
	}

	managed := filepath.Join(managedDir, "e2ectl")
	data, err := os.ReadFile(managed)
	if err != nil {
		t.Fatal(err)
	}
	first := string(data)
	for _, want := range []string{
		"# e2ectl:dev begin",
		"Host dev\n    HostName 203.0.113.10\n    Port 22\n    User ubuntu",
		"IdentityFile " + key,
		"# e2ectl:dev end",
		otherBlock,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("managed ssh entry is missing %q:\n%s", want, first)
		}
	}
	if info, err := os.Stat(managedDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("managed ssh dir must be 0700: %v %v", info, err)
	}
	if info, err := os.Stat(managed); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("managed ssh file must be 0600: %v %v", info, err)
	}

	main, err := os.ReadFile(mainConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(main); got != unrelated+sshIncludeLine+"\n" {
		t.Fatalf("main ssh config must keep its content and gain one Include line, got:\n%s", got)
	}

	// re-running connect on the same env replaces its own block in place and
	// changes nothing: byte-identical managed file and single Include line
	if err := Run([]string{"dev"}, store); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(managed)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != first {
		t.Fatalf("re-running connect on %q must be idempotent:\nbefore:\n%s\nafter:\n%s", "dev", first, data)
	}
	main, err = os.ReadFile(mainConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(main), sshIncludeLine) != 1 {
		t.Fatalf("the Include line must be appended exactly once, got:\n%s", main)
	}

	// a second environment gets its own block without disturbing the first
	addEnv(t, store, "dev2", "ec2-host", "",
		provisioner.RawResources{"remoteHost": []byte(`{"cloudProvider":"aws","address":"203.0.113.11","port":22,"username":"ubuntu"}`)})
	if err := Run([]string{"dev2"}, store); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(managed)
	if err != nil {
		t.Fatal(err)
	}
	withSecond := string(data)
	if !strings.Contains(withSecond, first) {
		t.Fatalf("adding a second environment must keep the first block untouched:\n%s", withSecond)
	}
	if !strings.Contains(withSecond, "# e2ectl:dev2 begin") {
		t.Fatalf("the second environment must get its own block:\n%s", withSecond)
	}

	// --print writes nothing, whichever side of the env name it sits on
	for _, args := range [][]string{{"--print", "dev2"}, {"dev2", "--print"}} {
		if err := Run(args, store); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(managed)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != withSecond {
			t.Fatalf("--print must not change the managed ssh entry (args %v):\n%s", args, data)
		}
	}
}

// Without a configured private key there is nothing safe to write: connect
// prints the one-off command and writes no ssh config at all.
func TestSSHPasswordAuthWritesNoConfig(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CI", "")
	t.Setenv("E2E_PROFILE", "")
	t.Setenv("E2E_AWS_PRIVATE_KEY_PATH", "")

	addEnv(t, store, "dev", "ec2-host", "",
		provisioner.RawResources{"remoteHost": []byte(`{"cloudProvider":"aws","address":"203.0.113.10","port":22,"username":"ubuntu"}`)})
	out := captureStdout(t, func() {
		if err := Run([]string{"dev"}, store); err != nil {
			t.Errorf("password auth is a warning, not a failure: %v", err)
		}
	})
	if !strings.Contains(out, "ssh -p 22 ubuntu@203.0.113.10") {
		t.Fatalf("the one-off ssh command must be printed, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "config.d")); !os.IsNotExist(err) {
		t.Fatal("password auth must not write an ssh config entry")
	}
}

// A docker-transport host is reached with docker exec; no ssh config applies.
func TestDockerTransportHost(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	addEnv(t, store, "dev", "docker-host", "http://127.0.0.1:30080",
		provisioner.RawResources{"remoteHost": []byte(`{"cloudProvider":"local","transport":"docker","address":"dev-agent"}`)})
	out := captureStdout(t, func() {
		if err := Run([]string{"dev"}, store); err != nil {
			t.Errorf("docker transport needs no configuration: %v", err)
		}
	})
	if !strings.Contains(out, "docker exec -it dev-agent bash") {
		t.Fatalf("the docker exec command must be printed, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh")); !os.IsNotExist(err) {
		t.Fatal("docker transport must not write ssh config")
	}
}

const envKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: kind-dev
contexts:
- context:
    cluster: kind-dev
    user: kind-dev
  name: kind-dev
current-context: kind-dev
preferences: {}
users:
- name: kind-dev
  user:
    client-certificate-data: Zm9v
    client-key-data: YmFy
`

const userKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://192.168.1.1:6443
  name: minikube
contexts:
- context:
    cluster: minikube
    user: minikube
  name: minikube
current-context: minikube
preferences: {}
users:
- name: minikube
  user:
    token: abc
`

// The context is renamed to the environment name, the cluster/user refs are
// kept, the merge preserves unrelated entries and never changes the current
// context, and re-running is idempotent.
func TestKubeconfigContextRenameAndMerge(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dest := filepath.Join(home, ".kube", "config")
	t.Setenv("KUBECONFIG", dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(userKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := addEnv(t, store, "dev", "kind", "http://127.0.0.1:30080", clusterSnapshot(envKubeconfig))
	if err := Run([]string{"dev"}, store); err != nil {
		t.Fatal(err)
	}

	envCfg, err := os.ReadFile(entry.KubeconfigPath())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := clientcmd.Load(envCfg)
	if err != nil {
		t.Fatalf("the environment kubeconfig must stay valid: %v\n%s", err, envCfg)
	}
	ctx, ok := parsed.Contexts["dev"]
	if !ok {
		t.Fatalf("the context must be renamed to the environment name:\n%s", envCfg)
	}
	if _, old := parsed.Contexts["kind-dev"]; old {
		t.Fatalf("the old context name must be gone:\n%s", envCfg)
	}
	if parsed.CurrentContext != "dev" {
		t.Fatalf("current-context must follow the rename, got %q", parsed.CurrentContext)
	}
	// the cluster and user references are kept, not renamed
	if ctx.Cluster != "kind-dev" || ctx.AuthInfo != "kind-dev" {
		t.Fatalf("the cluster/user refs must be kept, got cluster %q user %q", ctx.Cluster, ctx.AuthInfo)
	}

	merged, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: minikube", "name: dev\n", "current-context: minikube", "server: https://127.0.0.1:6443", "server: https://192.168.1.1:6443"} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("the merged kubeconfig is missing %q:\n%s", want, merged)
		}
	}
	if strings.Count(string(merged), "cluster: kind-dev") != 1 {
		t.Fatalf("the env context must appear once, got:\n%s", merged)
	}

	// idempotent: a second run does not duplicate entries
	if err := Run([]string{"dev"}, store); err != nil {
		t.Fatal(err)
	}
	mergedAgain, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(mergedAgain) != string(merged) {
		t.Fatalf("re-running connect must be idempotent:\nbefore:\n%s\nafter:\n%s", merged, mergedAgain)
	}
}

// --print leaves both files untouched.
func TestKubeconfigPrintWritesNothing(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dest := filepath.Join(home, ".kube", "config")
	t.Setenv("KUBECONFIG", dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(userKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := addEnv(t, store, "dev", "kind", "", clusterSnapshot(envKubeconfig))
	if err := Run([]string{"--print", "dev"}, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry.KubeconfigPath()); !os.IsNotExist(err) {
		t.Fatal("--print must not write the environment kubeconfig")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != userKubeconfig {
		t.Fatalf("--print must not touch the user kubeconfig: %q %v", data, err)
	}
}

// Multi-context kubeconfigs are rejected honestly — our bases produce
// single-context ones, and guessing is worse than failing.
func TestKubeconfigMultiContextRejected(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KUBECONFIG", filepath.Join(home, ".kube", "config"))
	two := strings.Replace(envKubeconfig,
		"contexts:\n- context:\n    cluster: kind-dev\n    user: kind-dev\n  name: kind-dev\n",
		"contexts:\n- context:\n    cluster: kind-dev\n    user: kind-dev\n  name: kind-dev\n- context:\n    cluster: kind-dev\n    user: kind-dev\n  name: extra\n", 1)
	entry := addEnv(t, store, "dev", "kind", "", clusterSnapshot(two))
	err := Run([]string{"dev"}, store)
	if err == nil || !strings.Contains(err.Error(), "2 contexts") {
		t.Fatalf("a multi-context kubeconfig must fail honestly, got: %v", err)
	}
	if _, statErr := os.Stat(entry.KubeconfigPath()); !os.IsNotExist(statErr) {
		t.Fatal("a rejected kubeconfig must not be written")
	}
}

// The local base prints docker commands and the connection card; unknown
// bases error naming themselves and the known ones; a missing environment
// name lists what exists.
func TestDispatchCardLocalAndUnknownBase(t *testing.T) {
	store := testStore(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	addEnv(t, store, "dev", "local", "http://127.0.0.1:30080", nil)
	out := captureStdout(t, func() {
		if err := Run([]string{"dev"}, store); err != nil {
			t.Errorf("the local base needs no configuration: %v", err)
		}
	})
	for _, want := range []string{
		"docker exec -it dev-agent bash",
		"dev-fakeintake",
		"fakeintake: http://127.0.0.1:30080",
		"env dir:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("the connection card is missing %q:\n%s", want, out)
		}
	}

	addEnv(t, store, "weird", "gcp", "", nil)
	err := Run([]string{"weird"}, store)
	if err == nil || !strings.Contains(err.Error(), "gcp") || !strings.Contains(err.Error(), knownBases) {
		t.Fatalf("an unknown base must error naming the base and the known ones, got: %v", err)
	}

	out = captureStdout(t, func() {
		if err := Run(nil, store); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("a missing environment name must be a usage error, got: %v", err)
		}
	})
	if !strings.Contains(out, "dev") {
		t.Fatalf("a missing environment name must list the available ones:\n%s", out)
	}
}
