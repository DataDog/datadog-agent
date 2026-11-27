// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"errors"
	"fmt"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

// cloneErr rebuilds the error chain that the eBPF manager returns when cloning a TC probe fails.
func cloneErr(cause error) error {
	pip := manager.ProbeIdentificationPair{UID: "security", EBPFFuncName: "classifier_egress"}
	return fmt.Errorf("couldn't clone %s: %w", pip, cause)
}

// linkNotFoundErr asks netlink for an interface that cannot exist: the error it returns wraps an
// unexported field, so this is the only way to get one from outside of the netlink package.
func linkNotFoundErr(t *testing.T) error {
	_, err := netlink.LinkByName("dd-no-such-link")
	require.ErrorAs(t, err, &netlink.LinkNotFoundError{})
	return err
}

// attachErr rebuilds the error chain that the eBPF manager returns when attaching a cloned TC probe fails.
func attachErr(cause error) error {
	pip := manager.ProbeIdentificationPair{UID: "security_classifier_egress_2_4026532420", EBPFFuncName: "classifier_egress"}
	return multierror.Append(nil, cloneErr(
		fmt.Errorf("failed to attach new probe %v: %w", pip,
			fmt.Errorf("couldn't start probe %v: %w", pip, cause)))).ErrorOrNil()
}

func tcxAttachErr(cause error) error {
	return fmt.Errorf("%w to %s[%d]: %w", manager.ErrTCXAttach, "eth0", 2, fmt.Errorf("attach tcx link: %w", cause))
}

func setnsErr(cause error) error {
	return fmt.Errorf("%w: couldn't enter the namespace: %w", manager.ErrNetnsSwitch, cause)
}

func TestClassifyTCClassifierError(t *testing.T) {
	pip := manager.ProbeIdentificationPair{UID: "security_classifier_egress_2_4026532420", EBPFFuncName: "classifier_egress"}

	for _, test := range []struct {
		name     string
		err      error
		expected string
	}{
		{
			name: "identification pair in use",
			err: multierror.Append(nil,
				cloneErr(fmt.Errorf("couldn't add probe %v: %w", pip, manager.ErrIdentificationPairInUse)),
				cloneErr(fmt.Errorf("couldn't add probe %v: %w", pip, manager.ErrIdentificationPairInUse)),
			).ErrorOrNil(),
			expected: errorClassClassifierExists,
		},
		{
			name: "link not found",
			err: fmt.Errorf("failed to attach new probe %v: %w", pip,
				fmt.Errorf("couldn't start probe %v: %w", pip,
					fmt.Errorf("couldn't resolve interface with IfIndex %d in namespace %d: %w", 1658, 4026536040,
						linkNotFoundErr(t)))),
			expected: errorClassLinkNotFound,
		},
		{
			name: "missing device",
			err: multierror.Append(nil, cloneErr(
				fmt.Errorf("failed to attach new probe %v: %w", pip,
					fmt.Errorf("couldn't start probe %v: %w", pip,
						fmt.Errorf("couldn't add a %q qdisc to interface %s[%d]: %w", "clsact", "tmp0e9ec", 231, unix.ENODEV))))).ErrorOrNil(),
			expected: errorClassNoSuchDevice,
		},
		{
			name: "missing device on filter add",
			err: multierror.Append(nil, cloneErr(
				fmt.Errorf("failed to attach new probe %v: %w", pip,
					fmt.Errorf("couldn't start probe %v: %w", pip,
						fmt.Errorf("couldn't add a %v filter to interface %s[%d]: %w", "ingress", "tmpc255a", 145, unix.ENODEV))))).ErrorOrNil(),
			expected: errorClassNoSuchDevice,
		},
		{
			name: "filter not found",
			err: multierror.Append(nil, cloneErr(
				fmt.Errorf("failed to attach new probe %v: %w", pip,
					fmt.Errorf("couldn't start probe %v: %w", pip,
						fmt.Errorf("couldn't create TC filter for %v: filter not found", pip))))).ErrorOrNil(),
			expected: errorClassFilterNotFound,
		},
		{
			name:     "unknown",
			err:      multierror.Append(nil, cloneErr(errors.New("something else"))).ErrorOrNil(),
			expected: errorClassUnknown,
		},
		{
			name: "link not found before a tcx attach",
			err: attachErr(fmt.Errorf("couldn't resolve interface with IfIndex %d in namespace %d: %w", 1658, 4026536040,
				linkNotFoundErr(t))),
			expected: errorClassLinkNotFound,
		},
		{
			name:     "missing device on tcx attach",
			err:      attachErr(tcxAttachErr(unix.ENODEV)),
			expected: errorClassNoSuchDevice,
		},
		{
			name:     "tcx hook full",
			err:      attachErr(tcxAttachErr(unix.ERANGE)),
			expected: errorClassTCXAttach,
		},
		{
			name:     "tcx attach denied",
			err:      attachErr(tcxAttachErr(unix.EPERM)),
			expected: errorClassTCXAttach,
		},
		{
			name:     "setns denied",
			err:      attachErr(setnsErr(unix.EPERM)),
			expected: errorClassSetns,
		},
		{
			name: "stale namespace handle",
			err: attachErr(fmt.Errorf("%w: handle %d refers to inode %d instead of namespace %d",
				manager.ErrNetnsSwitch, 12, 4026531840, 4026532420)),
			expected: errorClassSetns,
		},
		{
			name:     "namespace not restored after a tcx attach failure",
			err:      attachErr(errors.Join(tcxAttachErr(unix.EPERM), fmt.Errorf("%w: couldn't restore the original namespace: %w", manager.ErrNetnsSwitch, unix.EINVAL))),
			expected: errorClassSetns,
		},
		{
			name: "tcx attach failure next to an existing classifier",
			err: multierror.Append(nil,
				cloneErr(fmt.Errorf("couldn't add probe %v: %w", pip, manager.ErrIdentificationPairInUse)),
				attachErr(tcxAttachErr(unix.ERANGE)),
			).ErrorOrNil(),
			expected: errorClassTCXAttach,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			class := classifyTCClassifierError(test.err)
			assert.Equal(t, test.expected, class)
			// a class without a counter would make countError panic
			assert.Contains(t, errorClasses, class)
		})
	}
}

func TestErrorCounters(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	nr := &Resolver{client: client, errorCounters: newErrorCounters()}

	expected := make(map[string]int64, len(errorClasses))
	for _, class := range errorClasses {
		nr.countError(class)
		expected[class] = 1
	}
	nr.countError(errorClassLinkNotFound)
	expected[errorClassLinkNotFound] = 2

	nr.sendErrorStats()

	prefix := metrics.MetricNamespaceResolverError + ":error_type:"
	assert.Equal(t, expected, client.GetByPrefix(prefix))

	// the flush resets the counters, so nothing is reported until a new error is counted
	nr.sendErrorStats()
	assert.Equal(t, expected, client.GetByPrefix(prefix))
}
