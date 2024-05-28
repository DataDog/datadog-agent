// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func init() {
	rand.Seed(10)
}

func TestMinBackoffFactorValid(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	// Verify default
	defaultValue := policy.MinBackoffFactor
	assert.Equal(t, float64(2), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetWithoutSource("forwarder_backoff_factor", 4)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(4), policy.MinBackoffFactor)

	// Verify invalid values recover gracefully
	mockConfig.SetWithoutSource("forwarder_backoff_factor", 1.5)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.MinBackoffFactor)
}

func TestBaseBackoffTimeValid(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.BaseBackoffTime
	assert.Equal(t, float64(2), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetWithoutSource("forwarder_backoff_base", 4)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(4), policy.BaseBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.SetWithoutSource("forwarder_backoff_base", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.BaseBackoffTime)
}

func TestMaxBackoffTimeValid(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.MaxBackoffTime
	assert.Equal(t, float64(64), defaultValue)

	// Verify configuration updates global var
	mockConfig.SetWithoutSource("forwarder_backoff_max", 128)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, float64(128), policy.MaxBackoffTime)

	// Verify invalid values recover gracefully
	mockConfig.SetWithoutSource("forwarder_backoff_max", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.MaxBackoffTime)
}

func TestRecoveryIntervalValid(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	// Verify default
	defaultValue := policy.RecoveryInterval
	recoveryReset := pkgconfigsetup.Datadog().GetBool("forwarder_recovery_reset")
	assert.Equal(t, 2, defaultValue)
	assert.Equal(t, false, recoveryReset)

	// Verify configuration updates global var
	mockConfig.SetWithoutSource("forwarder_recovery_interval", 1)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, 1, policy.RecoveryInterval)

	// Verify invalid values recover gracefully
	mockConfig.SetWithoutSource("forwarder_recovery_interval", 0)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, defaultValue, policy.RecoveryInterval)

	// Verify reset error count
	mockConfig.SetWithoutSource("forwarder_recovery_reset", true)
	e = newBlockedEndpoints(mockConfig, log)
	policy, ok = e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)
	assert.Equal(t, policy.MaxErrors, policy.RecoveryInterval)
}

// Test we increase delay on average
func TestGetBackoffDurationIncrease(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)
	previousBackoffDuration := time.Duration(0) * time.Second
	backoffIncrease := 0
	backoffDecrease := 0

	for i := 1; ; i++ {
		backoffDuration := e.getBackoffDuration(i)

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if backoffDuration == previousBackoffDuration {
			break
		} else {
			if backoffDuration > previousBackoffDuration {
				backoffIncrease++
			} else {
				backoffDecrease++
			}
			previousBackoffDuration = backoffDuration
		}
	}

	assert.True(t, backoffIncrease >= backoffDecrease)
}

func TestMaxGetBackoffDuration(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)
	backoffDuration := e.getBackoffDuration(100)

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	assert.Equal(t, time.Duration(policy.MaxBackoffTime)*time.Second, backoffDuration)
}

func TestMaxErrors(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)
	previousBackoffDuration := time.Duration(0) * time.Second
	attempts := 0

	for i := 1; ; i++ {
		backoffDuration := e.getBackoffDuration(i)

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if backoffDuration == previousBackoffDuration {
			attempts = i - 1
			break
		}

		previousBackoffDuration = backoffDuration
	}

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	assert.Equal(t, policy.MaxErrors, attempts)
}

func TestBlock(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	e.close("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, now.Before(e.errorPerEndpoint["test"].until))
}

func TestMaxBlock(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)
	e.close("test")
	e.errorPerEndpoint["test"].nbError = 1000000

	e.close("test")
	now := time.Now()

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	maxBackoffDuration := time.Duration(policy.MaxBackoffTime) * time.Second

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.Equal(t, policy.MaxErrors, e.errorPerEndpoint["test"].nbError)
	assert.True(t, now.Add(maxBackoffDuration).After(e.errorPerEndpoint["test"].until) ||
		now.Add(maxBackoffDuration).Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblock(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	e.close("test")
	require.Contains(t, e.errorPerEndpoint, "test")
	e.close("test")
	e.close("test")
	e.close("test")
	e.close("test")

	e.recover("test")

	policy, ok := e.backoffPolicy.(*backoff.ExpBackoffPolicy)
	assert.True(t, ok)

	assert.True(t, e.errorPerEndpoint["test"].nbError == int(math.Max(0, float64(5-policy.RecoveryInterval))))
}

func TestMaxUnblock(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	e.close("test")
	e.recover("test")
	e.recover("test")
	now := time.Now()

	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
	assert.True(t, now.After(e.errorPerEndpoint["test"].until) || now.Equal(e.errorPerEndpoint["test"].until))
}

func TestUnblockUnknown(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	e.recover("test")
	assert.Contains(t, e.errorPerEndpoint, "test")
	assert.True(t, e.errorPerEndpoint["test"].nbError == 0)
}

func TestIsBlock(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlock("test"))

	e.close("test")
	assert.True(t, e.isBlock("test"))

	e.recover("test")
	assert.False(t, e.isBlock("test"))
}

func TestIsBlockTiming(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	// setting an old close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(-30 * time.Second)}
	assert.False(t, e.isBlock("test"))

	// setting an new close
	e.errorPerEndpoint["test"] = &block{nbError: 1, until: time.Now().Add(30 * time.Second)}
	assert.True(t, e.isBlock("test"))
}

func TestIsblockUnknown(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	e := newBlockedEndpoints(mockConfig, log)

	assert.False(t, e.isBlock("test"))
}
