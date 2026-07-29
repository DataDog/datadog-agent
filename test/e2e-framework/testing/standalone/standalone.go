// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package standalone provides helpers to drive the e2e provisioning framework from
// standalone (non-test) binaries. It mirrors the provisioning performed by
// e2e.BaseSuite but without any dependency on *testing.T (see PR #51954), so a plain
// program can provision an environment, use it, and tear it down.
package standalone

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

const (
	createTimeout = 60 * time.Minute
	deleteTimeout = 30 * time.Minute
)

// Context is a non-test implementation of [common.Context], usable from standalone binaries.
type Context struct {
	logger    *log.Logger
	outputDir string
}

var _ common.Context = (*Context)(nil)

// NewContext returns a [Context] that logs to stderr and stores session output in outputDir.
func NewContext(outputDir string) *Context {
	return &Context{
		logger:    log.New(os.Stderr, "", log.LstdFlags),
		outputDir: outputDir,
	}
}

// T returns nil: there is no *testing.T outside of tests. The client and component layers
// no longer call T() (see PR #51954), so a nil value is safe for the provisioning path.
func (c *Context) T() *testing.T { return nil }

// Logf logs a formatted message.
func (c *Context) Logf(format string, args ...any) { c.logger.Printf(format, args...) }

// FailNow logs the formatted message and panics, stopping the current goroutine.
func (c *Context) FailNow(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	c.logger.Print(msg)
	panic(msg)
}

// SessionOutputDir returns the directory where output files and artifacts are stored.
func (c *Context) SessionOutputDir() string { return c.outputDir }

// ctxLogWriter adapts a [common.Context] to an [io.Writer] so it can be passed to provisioners.
type ctxLogWriter struct{ ctx common.Context }

func (w ctxLogWriter) Write(p []byte) (int, error) {
	w.ctx.Logf("%s", string(p))
	return len(p), nil
}

func newLogWriter(ctx common.Context) io.Writer { return ctxLogWriter{ctx: ctx} }

// Provision provisions an environment of type Env using the given provisioner and stack name,
// imports the resulting resources into the environment, and initializes it. It mirrors the
// provisioning performed by e2e.BaseSuite, without any dependency on *testing.T.
//
// The returned environment is ready to use (e.g. env.RemoteHost.Execute). Callers are
// responsible for calling [Destroy] when done.
func Provision[Env any](ctx common.Context, stackName string, p provisioners.Provisioner) (*Env, error) {
	env, _, err := ProvisionWithResources[Env](ctx, stackName, p)
	return env, err
}

// ProvisionWithResources is like [Provision] but also returns the raw stack outputs so
// that callers can persist them without a second Pulumi read.
func ProvisionWithResources[Env any](ctx common.Context, stackName string, p provisioners.Provisioner) (*Env, provisioners.RawResources, error) {
	pCtx, cancel := context.WithTimeout(context.Background(), createTimeout)
	defer cancel()

	logger := newLogWriter(ctx)

	env, fields, values, err := environments.CreateEnv[Env]()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create env %T for stack %s: %w", env, stackName, err)
	}

	var resources provisioners.RawResources
	switch pType := p.(type) {
	case provisioners.TypedProvisioner[Env]:
		resources, err = pType.ProvisionEnv(pCtx, stackName, logger, env)
	case provisioners.UntypedProvisioner:
		resources, err = pType.Provision(pCtx, stackName, logger)
	default:
		return nil, nil, fmt.Errorf("provisioner of type %T implements neither TypedProvisioner nor UntypedProvisioner", p)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("provisioning stack %s with provisioner %s failed: %w", stackName, p.ID(), err)
	}

	// Refresh field values from env to capture any changes made by the provisioner
	// (e.g. fields set to nil when certain components are not deployed).
	envValue := reflect.ValueOf(env)
	for idx, field := range fields {
		values[idx] = envValue.Elem().FieldByIndex(field.Index)
	}

	if err := environments.BuildEnvFromResources(ctx, resources, fields, values); err != nil {
		return nil, nil, fmt.Errorf("unable to build env %T from resources for stack %s: %w", env, stackName, err)
	}

	if initializable, ok := any(env).(common.Initializable); ok {
		if err := initializable.Init(ctx); err != nil {
			return nil, nil, fmt.Errorf("failed to init environment: %w", err)
		}
	}

	return env, resources, nil
}

// HydrateFromResources builds an environment of type Env from already-captured
// RawResources (e.g. persisted at create time) and the import-key mapping
// captured by [environments.ImportKeys] at provision time, with NO Pulumi
// interaction.
//
// For each importable field in Env:
//   - If keys[FieldName] is present, the key is replayed via SetKey before
//     calling [environments.BuildEnvFromResources], so the resource is found by
//     the correct key in the resources map.
//   - If keys[FieldName] is absent, the field is set to nil so
//     [environments.BuildEnvFromResources] treats it as an optional component
//     that was not deployed.
func HydrateFromResources[Env any](ctx common.Context, resources provisioners.RawResources, keys map[string]string) (*Env, error) {
	env, fields, values, err := environments.CreateEnv[Env]()
	if err != nil {
		return nil, fmt.Errorf("unable to create env %T from cached resources: %w", env, err)
	}

	// Refresh field values from env (mirrors Provision's pattern) and apply
	// the captured import keys so BuildEnvFromResources can match resources.
	envValue := reflect.ValueOf(env)
	for idx, field := range fields {
		values[idx] = envValue.Elem().FieldByIndex(field.Index)
		if k, ok := keys[field.Name]; ok {
			// Key present — replay it on the importable so the resource lookup
			// uses the correct export name.
			values[idx].Interface().(components.Importable).SetKey(k)
		} else {
			// Key absent — mark this field as nil so BuildEnvFromResources
			// treats it as a component that was not deployed.
			values[idx].Set(reflect.Zero(values[idx].Type()))
		}
	}

	if err := environments.BuildEnvFromResources(ctx, resources, fields, values); err != nil {
		return nil, fmt.Errorf("unable to build env %T from cached resources: %w", env, err)
	}

	if initializable, ok := any(env).(common.Initializable); ok {
		if err := initializable.Init(ctx); err != nil {
			return nil, fmt.Errorf("failed to init environment from cached resources: %w", err)
		}
	}

	return env, nil
}

// Destroy tears down the stack provisioned for stackName using the given provisioner.
func Destroy(ctx common.Context, stackName string, p provisioners.Provisioner) error {
	pCtx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
	defer cancel()
	return p.Destroy(pCtx, stackName, newLogWriter(ctx))
}
