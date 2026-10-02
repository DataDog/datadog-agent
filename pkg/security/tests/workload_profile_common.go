// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// dedicatedWorkloadProfileNodeEnv is set by the KMT job dedicated to the workload profile tests
const dedicatedWorkloadProfileNodeEnv = "DEDICATED_WORKLOAD_PROFILE_NODE"

// IsDedicatedNodeForWorkloadProfile returns true when the tests run on the node dedicated to the workload profile tests
func IsDedicatedNodeForWorkloadProfile() bool {
	_, present := os.LookupEnv(dedicatedWorkloadProfileNodeEnv)
	return present
}

// skipIfNoWorkloadProfileEnv skips the test unless it runs on the dedicated node, with docker available
func skipIfNoWorkloadProfileEnv(t *testing.T) {
	t.Helper()
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForWorkloadProfile() {
		t.Skip("Skip test when not run in dedicated env")
	}
}

// workloadProfileTestOpts returns the static options shared by the workload profile tests: profiles and anomaly
// detection enabled, anomalies sent as soon as a profile starts, a storage directory dedicated to the test and a
// tagger that lets the test pick the image name and tag of each container.
func workloadProfileTestOpts(storageDir string, tagger *FakeManualTagger) testOpts {
	return testOpts{
		enableSecurityProfile:     true,
		enableAnomalyDetection:    true,
		workloadProfileStorageDir: storageDir,
		tagger:                    tagger,
	}
}

// getManagerV2 returns the workload profile manager of the test module
func getManagerV2(test *testModule) (*securityprofile.ManagerV2, error) {
	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return nil, errors.New("not supported")
	}
	m, ok := p.GetProfileManager().(*securityprofile.ManagerV2)
	if !ok {
		return nil, errors.New("the workload profile manager isn't running")
	}
	return m, nil
}

var workloadProfileImageCounter atomic.Uint64

// newWorkloadProfileImage returns an image name that no other container of the test suite uses
func newWorkloadProfileImage() string {
	return fmt.Sprintf("workload-profile-image-%d", workloadProfileImageCounter.Add(1))
}

const (
	// workloadProfileSyscallTester is the path of the syscall tester inside the containers
	workloadProfileSyscallTester = "/usr/local/bin/wp-syscall-tester"
)

// workloadProfileExecVariants lists the variants of workloadProfileExecPath copied in each container
var workloadProfileExecVariants = []string{"known", "new", "args", "syscalls", "capabilities"}

// workloadProfileExecPath returns the path, inside the containers, of the copy of the syscall tester dedicated to
// the variant: a dedicated binary per variant gives a dedicated process node per variant
func workloadProfileExecPath(variant string) string {
	return "/usr/local/bin/wp-exec-" + variant
}

// startWorkloadProfileContainer starts a container that the tagger resolves to the provided image name and tag,
// and returns the selector of the profile of this image: a profile is shared by all the tags of an image.
// The binaries used by the activities are copied inside the container, so that their paths don't depend on the
// test module and stay the same across module restarts.
func startWorkloadProfileContainer(t *testing.T, test *testModule, tagger *FakeManualTagger, syscallTester string, image string, tag string) (*dockerCmdWrapper, cgroupModel.WorkloadSelector) {
	t.Helper()
	tagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{Image: image, Tag: tag})

	dockerInstance, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = dockerInstance.stop()
	})

	copyToContainer := func(dst string) {
		cmd := exec.Command(dockerInstance.executable, "cp", syscallTester, dockerInstance.containerName+":"+dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("couldn't copy %s to %s: %v (%s)", syscallTester, dst, err, string(out))
		}
	}
	copyToContainer(workloadProfileSyscallTester)
	for _, variant := range workloadProfileExecVariants {
		copyToContainer(workloadProfileExecPath(variant))
	}

	return dockerInstance, cgroupModel.WorkloadSelector{Image: image, Tag: "*"}
}

// waitForWorkloadProfile waits until the profile of the selector satisfies check, and returns its snapshot
func waitForWorkloadProfile(t *testing.T, test *testModule, selector cgroupModel.WorkloadSelector, check func(p *profile.Profile) bool) (*profile.Profile, error) {
	t.Helper()
	m, err := getManagerV2(test)
	if err != nil {
		return nil, err
	}

	var snapshot *profile.Profile
	err = retry(t, func() error {
		snapshot, err = m.GetProfileSnapshot(selector)
		if err != nil {
			return backoff.Permanent(err)
		}
		if snapshot == nil {
			return fmt.Errorf("no profile for selector %s", selector.String())
		}
		if !check(snapshot) {
			return fmt.Errorf("the profile of selector %s doesn't match the expected content", selector.String())
		}
		return nil
	}, backoff.WithBackOff(backoff.NewConstantBackOff(time.Second)), backoff.WithMaxTries(15))
	return snapshot, err
}

// getWorkloadProfile returns the current snapshot of the profile of the selector, or nil if the manager has none
func getWorkloadProfile(t *testing.T, test *testModule, selector cgroupModel.WorkloadSelector) *profile.Profile {
	t.Helper()
	m, err := getManagerV2(test)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.GetProfileSnapshot(selector)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// workloadProfileAnomaly is an anomaly detection event sent while running an action
type workloadProfileAnomaly struct {
	eventType model.EventType
	json      string
}

var sentinelCounter atomic.Uint64

// runUntilSentinel runs action, then executes a sentinel command in the container and returns once the sentinel
// exec event has been dispatched. Events are dispatched in order and the workload profile manager processes each
// event, and sends the anomaly it triggers, before the event reaches the probe event handlers: once the sentinel
// is seen, every event of the action and every anomaly it triggered has already been delivered.
// onProbeEvent, if not nil, is called for each dispatched event until the sentinel.
func runUntilSentinel(t *testing.T, test *testModule, dockerInstance *dockerCmdWrapper, action func() error, onProbeEvent func(event *model.Event)) {
	t.Helper()

	token := fmt.Sprintf("test-sentinel-%d", sentinelCounter.Add(1))
	seen := make(chan struct{})
	var once sync.Once
	test.RegisterProbeEventHandler(func(event *model.Event) {
		if onProbeEvent != nil {
			onProbeEvent(event)
		}
		if event.GetEventType() != model.ExecEventType {
			return
		}
		if args, _ := event.GetFieldValue("exec.args"); args == token {
			once.Do(func() { close(seen) })
		}
	})
	defer test.RegisterProbeEventHandler(nil)

	if err := action(); err != nil {
		t.Fatal(err)
	}

	if out, err := dockerInstance.Command("echo", []string{token}, []string{}).CombinedOutput(); err != nil {
		t.Fatalf("couldn't run the sentinel: %v (%s)", err, string(out))
	}
	select {
	case <-seen:
	case <-time.After(getEventTimeout):
		t.Fatalf("the sentinel exec event %s wasn't received within %s", token, getEventTimeout)
	}
}

// collectAnomalies runs action in the container and returns the anomaly detection events it triggered
func collectAnomalies(t *testing.T, test *testModule, action func() error, dockerInstance *dockerCmdWrapper) []workloadProfileAnomaly {
	t.Helper()

	var (
		lock      sync.Mutex
		anomalies []workloadProfileAnomaly
	)
	test.RegisterCustomSendEventHandler(func(rule *rules.Rule, event *events.CustomEvent) {
		if rule.ID != events.AnomalyDetectionRuleID {
			return
		}
		data, err := event.MarshalJSON()
		if err != nil {
			t.Errorf("couldn't marshal anomaly detection event: %v", err)
			return
		}
		lock.Lock()
		anomalies = append(anomalies, workloadProfileAnomaly{eventType: event.GetEventType(), json: string(data)})
		lock.Unlock()
	})
	defer test.RegisterCustomSendEventHandler(nil)

	runUntilSentinel(t, test, dockerInstance, action, nil)

	lock.Lock()
	defer lock.Unlock()
	return append([]workloadProfileAnomaly(nil), anomalies...)
}

// workloadProfileEventType describes how to produce, in a container, an activity of a given event type, and how to
// find it back in a profile and in an anomaly detection event. An activity is identified by a variant name: the same
// variant always produces the same activity, and two variants produce two distinct entries in a profile.
type workloadProfileEventType struct {
	name      string
	eventType model.EventType
	// unsupported returns true on the kernels where the event type can't be captured
	unsupported func(kv *kernel.Version) bool
	// trigger produces the activity of the variant in the container
	trigger func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string)
	// hasNode returns true if the profile contains the entry of the variant
	hasNode func(p *profile.Profile, variant string) bool
	// matchAnomaly returns true if the anomaly was triggered by the activity of the variant
	matchAnomaly func(anomaly workloadProfileAnomaly, variant string) bool
}

// skipIfUnsupported skips the test on the kernels where the event type can't be captured
func (et workloadProfileEventType) skipIfUnsupported(t *testing.T) {
	t.Helper()
	if et.unsupported != nil {
		checkKernelCompatibility(t, et.name+" events not supported", et.unsupported)
	}
}

// supportedWorkloadProfileEventTypes returns the event types of workloadProfileEventTypes that the kernel supports
func supportedWorkloadProfileEventTypes(t *testing.T) []workloadProfileEventType {
	t.Helper()
	kv, err := kernel.NewKernelVersion()
	if err != nil {
		t.Fatal(err)
	}
	var supported []workloadProfileEventType
	for _, et := range workloadProfileEventTypes() {
		if et.unsupported != nil && et.unsupported(kv) {
			t.Logf("%s events not supported on this kernel", et.name)
			continue
		}
		supported = append(supported, et)
	}
	return supported
}

// hasAnomalyFor returns true if one of the anomalies was triggered by the activity of the variant
func (et workloadProfileEventType) hasAnomalyFor(anomalies []workloadProfileAnomaly, variant string) bool {
	for _, anomaly := range anomalies {
		if anomaly.eventType == et.eventType && et.matchAnomaly(anomaly, variant) {
			return true
		}
	}
	return false
}

// workloadProfileDNSDomains maps each variant to a domain name. The domains don't share any label so that they
// can't be merged by the profile DNS matching depth.
var workloadProfileDNSDomains = map[string]string{
	"known": "one.one.one.one",
	"new":   "dns.google",
}

// workloadProfilePorts maps each variant to the port used by the bind and connect activities
var workloadProfilePorts = map[string]int{
	"known": 4251,
	"new":   4252,
}

// workloadProfileEventTypes returns the event types supported by default by the workload profiles
func workloadProfileEventTypes() []workloadProfileEventType {
	runInContainer := func(t *testing.T, dockerInstance *dockerCmdWrapper, bin string, args ...string) {
		t.Helper()
		cmd := dockerInstance.Command(bin, args, []string{})
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("%s %v: %v (%s)", bin, args, err, string(out))
		}
	}

	return []workloadProfileEventType{
		{
			name:      "open",
			eventType: model.FileOpenEventType,
			trigger: func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string) {
				// busybox touch doesn't open a file that already exists, the syscall tester always does
				runInContainer(t, dockerInstance, workloadProfileSyscallTester, "open", "/tmp/wp-open-"+variant)
			},
			hasNode: func(p *profile.Profile, variant string) bool {
				return profileHasFile(p, "wp-open-"+variant)
			},
			matchAnomaly: func(anomaly workloadProfileAnomaly, variant string) bool {
				return strings.Contains(anomaly.json, "wp-open-"+variant+`"`)
			},
		},
		{
			name:      "exec",
			eventType: model.ExecEventType,
			trigger: func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string) {
				runInContainer(t, dockerInstance, workloadProfileExecPath(variant), "sleep", "0")
			},
			hasNode: func(p *profile.Profile, variant string) bool {
				return profileHasProcess(p, "wp-exec-"+variant)
			},
			matchAnomaly: func(anomaly workloadProfileAnomaly, variant string) bool {
				return strings.Contains(anomaly.json, "wp-exec-"+variant+`"`)
			},
		},
		{
			name:      "dns",
			eventType: model.DNSEventType,
			unsupported: func(kv *kernel.Version) bool {
				// TODO: Oracle because we are missing offsets. See dns_test.go
				return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
			},
			trigger: func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string) {
				runInContainer(t, dockerInstance, "nslookup", workloadProfileDNSDomains[variant])
			},
			hasNode: func(p *profile.Profile, variant string) bool {
				return profileHasDNS(p, workloadProfileDNSDomains[variant])
			},
			matchAnomaly: func(anomaly workloadProfileAnomaly, variant string) bool {
				return strings.Contains(anomaly.json, `"`+workloadProfileDNSDomains[variant]+`"`)
			},
		},
		{
			name:      "bind",
			eventType: model.BindEventType,
			trigger: func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string) {
				runInContainer(t, dockerInstance, workloadProfileSyscallTester, "bind", "AF_INET", "any", "udp", strconv.Itoa(workloadProfilePorts[variant]))
			},
			hasNode: func(p *profile.Profile, variant string) bool {
				return profileHasSocket(p, uint16(workloadProfilePorts[variant]), true)
			},
			matchAnomaly: func(anomaly workloadProfileAnomaly, variant string) bool {
				return strings.Contains(anomaly.json, fmt.Sprintf(`"port":%d`, workloadProfilePorts[variant]))
			},
		},
		{
			name:      "connect",
			eventType: model.ConnectEventType,
			trigger: func(t *testing.T, dockerInstance *dockerCmdWrapper, variant string) {
				// a UDP connect succeeds without any listener, a failed connect isn't added to the profile
				runInContainer(t, dockerInstance, workloadProfileSyscallTester, "connect", "AF_INET", "any", "udp", strconv.Itoa(workloadProfilePorts[variant]))
			},
			hasNode: func(p *profile.Profile, variant string) bool {
				return profileHasSocket(p, uint16(workloadProfilePorts[variant]), false)
			},
			matchAnomaly: func(anomaly workloadProfileAnomaly, variant string) bool {
				return strings.Contains(anomaly.json, fmt.Sprintf(`"port":%d`, workloadProfilePorts[variant]))
			},
		},
	}
}

// walkProfileProcesses returns the process nodes of the profile matching the filter
func walkProfileProcesses(p *profile.Profile, filter func(node *activity_tree.ProcessNode) bool) []*activity_tree.ProcessNode {
	return WalkActivityTree(p.ActivityTree, func(node *ProcessNodeAndParent) bool {
		return filter(node.Node)
	})
}

// profileHasProcess returns true if the profile contains a process node executing a file with the provided name
func profileHasProcess(p *profile.Profile, basename string) bool {
	return len(walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
		return filepath.Base(node.Process.FileEvent.PathnameStr) == basename
	})) > 0
}

// profileHasFile returns true if the profile contains a file node with the provided name
func profileHasFile(p *profile.Profile, basename string) bool {
	var hasFile func(files map[string]*activity_tree.FileNode) bool
	hasFile = func(files map[string]*activity_tree.FileNode) bool {
		for name, file := range files {
			if name == basename || hasFile(file.Children) {
				return true
			}
		}
		return false
	}
	return len(walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
		return hasFile(node.Files)
	})) > 0
}

// profileHasDNS returns true if the profile contains a DNS node for the provided domain
func profileHasDNS(p *profile.Profile, domain string) bool {
	return len(walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
		_, ok := node.DNSNames[domain]
		return ok
	})) > 0
}

// profileHasSocket returns true if the profile contains an AF_INET bind (or connect) node on the provided port
func profileHasSocket(p *profile.Profile, port uint16, bind bool) bool {
	return len(walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
		for _, sock := range node.Sockets {
			if sock.Family != "AF_INET" {
				continue
			}
			if bind {
				for _, b := range sock.Bind {
					if b.Port == port {
						return true
					}
				}
				continue
			}
			for _, c := range sock.Connect {
				if c.Port == port {
					return true
				}
			}
		}
		return false
	})) > 0
}
