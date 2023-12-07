// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/template"

	"go.uber.org/fx"
	yaml "gopkg.in/yaml.v2"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type provides struct {
	fx.Out

	Comp          secrets.Component
	FlareProvider flaretypes.Provider
}

type dependencies struct {
	fx.In

	Params secrets.Params
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSecretResolverProvider))
}

type handleToContext map[string][]secretContext

type secretResolver struct {
	enabled bool
	cache   map[string]string
	// list of handles and where they were found
	origin handleToContext

	backendCommand          string
	backendArguments        []string
	backendTimeout          int
	commandAllowGroupExec   bool
	removeTrailingLinebreak bool
	// responseMaxSize defines max size of the JSON output from a secrets reader backend
	responseMaxSize int

	// can be overridden for testing purposes
	commandHookFunc func(string) ([]byte, error)
	fetchHookFunc   func([]string) (map[string]string, error)
	scrubHookFunc   func([]string)
}

var _ secrets.Component = (*secretResolver)(nil)

//go:embed info.tmpl
var secretInfoTmpl string

type secretContext struct {
	// origin is the configuration name where a handle was found
	origin string
	// yamlPath is the key associated to the secret in the YAML configuration.
	// Example: in this yaml: '{"token": "ENC[token 1]"}', 'token' is the yamlPath and 'token 1' is the handle.
	yamlPath string
}

// TODO: (components) Hack to maintain a singleton reference to the secrets Component
//
// Only needed temporarily, since the secrets.Component is needed for the diagnose functionality.
// It is very difficult right now to modify diagnose because it would require modifying many
// function signatures, which would only increase future maintenance. Once diagnose is better
// integrated with Components, we should be able to remove this hack.
//
// Other components should not copy this pattern, it is only meant to be used temporarily.
var mu sync.Mutex
var instance *secretResolver

func newEnabledSecretResolver() *secretResolver {
	return &secretResolver{
		cache:   make(map[string]string),
		origin:  make(handleToContext),
		enabled: true,
	}
}

func newSecretResolverProvider(deps dependencies) provides {
	resolver := newEnabledSecretResolver()
	resolver.enabled = deps.Params.Enabled

	mu.Lock()
	defer mu.Unlock()
	if instance == nil {
		instance = resolver
	}

	return provides{
		Comp:          resolver,
		FlareProvider: flaretypes.NewProvider(resolver.fillFlare),
	}
}

// GetInstance returns the singleton instance of the secret.Component
func GetInstance() secrets.Component {
	mu.Lock()
	defer mu.Unlock()
	if instance == nil {
		deps := dependencies{Params: secrets.Params{Enabled: false}}
		p := newSecretResolverProvider(deps)
		instance = p.Comp.(*secretResolver)
	}
	return instance
}

// fillFlare add the inventory payload to flares.
func (r *secretResolver) fillFlare(fb flaretypes.FlareBuilder) error {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	r.GetDebugInfo(writer)
	writer.Flush()
	fb.AddFile("secrets.log", buffer.Bytes())
	return nil
}

func (r *secretResolver) registerSecretOrigin(handle string, origin string, yamlPath []string) {
	path := strings.Join(yamlPath, "/")
	for _, info := range r.origin[handle] {
		if info.origin == origin && info.yamlPath == path {
			// The secret was used twice in the same configuration under the same key: nothing to do
			return
		}
	}

	if len(yamlPath) != 0 {
		lastElem := yamlPath[len(yamlPath)-1:]
		if r.scrubHookFunc != nil {
			// hook used only for tests
			r.scrubHookFunc(lastElem)
		} else {
			scrubber.AddStrippedKeys(lastElem)
		}
	}

	r.origin[handle] = append(
		r.origin[handle],
		secretContext{
			origin:   origin,
			yamlPath: path,
		})
}

// Configure initializes the executable command and other options of the secrets component
func (r *secretResolver) Configure(command string, arguments []string, timeout, maxSize int, groupExecPerm, removeLinebreak bool) {
	if !r.enabled {
		return
	}
	r.backendCommand = command
	r.backendArguments = arguments
	r.backendTimeout = timeout
	if r.backendTimeout == 0 {
		r.backendTimeout = SecretBackendTimeoutDefault
	}
	r.responseMaxSize = maxSize
	if r.responseMaxSize == 0 {
		r.responseMaxSize = SecretBackendOutputMaxSizeDefault
	}
	r.commandAllowGroupExec = groupExecPerm
	r.removeTrailingLinebreak = removeLinebreak
	if r.commandAllowGroupExec {
		log.Warnf("Agent configuration relax permissions constraint on the secret backend cmd, Group can read and exec")
	}
}

func isEnc(str string) (bool, string) {
	// trimming space and tabs
	str = strings.Trim(str, " 	")
	if strings.HasPrefix(str, "ENC[") && strings.HasSuffix(str, "]") {
		return true, str[4 : len(str)-1]
	}
	return false, ""
}

// Resolve replaces all encrypted secrets in data by executing "secret_backend_command" once if all secrets aren't
// present in the cache.
func (r *secretResolver) Resolve(data []byte, origin string) ([]byte, error) {
	return r.resolve(data, origin, nil)
}

// ResolveWithCallback resolves the secrets in the given yaml data calling the callback with the YAML path of
// the secret handle and its value
func (r *secretResolver) ResolveWithCallback(data []byte, origin string, cb secrets.ResolveCallback) error {
	_, err := r.resolve(data, origin, cb)
	return err
}

func (r *secretResolver) resolve(data []byte, origin string, notifyCb secrets.ResolveCallback) ([]byte, error) {
	if !r.enabled {
		log.Infof("Agent secrets is disabled by caller")
		return nil, nil
	}
	if data == nil || r.backendCommand == "" {
		return data, nil
	}

	var config interface{}
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("could not Unmarshal config: %s", err)
	}

	// First we collect all new handles in the config
	newHandles := []string{}
	haveSecret := false

	w := &walker{
		resolver: func(yamlPath []string, value string) (string, error) {
			if ok, handle := isEnc(value); ok {
				haveSecret = true
				// Check if we already know this secret
				if secret, ok := r.cache[handle]; ok {
					log.Debugf("Secret '%s' was retrieved from cache", handle)
					// keep track of place where a handle was found
					r.registerSecretOrigin(handle, origin, yamlPath)
					return secret, nil
				}
				newHandles = append(newHandles, handle)
				return value, nil
			}
			return value, nil
		},
		notifier: notifyCb,
	}

	if err := w.walk(&config); err != nil {
		return nil, err
	}

	// the configuration does not contain any secrets
	if !haveSecret {
		return data, nil
	}

	// check if any new secrets need to be fetch
	if len(newHandles) != 0 {
		var secrets map[string]string
		var err error
		if r.fetchHookFunc != nil {
			// hook used only for tests
			secrets, err = r.fetchHookFunc(newHandles)
		} else {
			secrets, err = r.fetchSecret(newHandles)
		}
		if err != nil {
			return nil, err
		}

		w.resolver = func(yamlPath []string, value string) (string, error) {
			if ok, handle := isEnc(value); ok {
				if secret, ok := secrets[handle]; ok {
					log.Debugf("Secret '%s' was successfully resolved", handle)
					// keep track of place where a handle was found
					r.registerSecretOrigin(handle, origin, yamlPath)
					return secret, nil
				}

				// This should never happen since fetchSecret will return an error if not every handle have
				// been fetched.
				return "", fmt.Errorf("unknown secret '%s'", handle)
			}
			return value, nil
		}

		// Replace all newly resolved secrets in the config
		if err := w.walk(&config); err != nil {
			return nil, err
		}
	}

	finalConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not Marshal config after replacing encrypted secrets: %s", err)
	}
	return finalConfig, nil
}

type secretInfo struct {
	Executable                   string
	ExecutablePermissions        string
	ExecutablePermissionsDetails interface{}
	ExecutablePermissionsError   string
	Handles                      map[string][][]string
}

// GetDebugInfo exposes debug informations about secrets to be included in a flare
func (r *secretResolver) GetDebugInfo(w io.Writer) {
	if !r.enabled {
		fmt.Fprintf(w, "Agent secrets is disabled by caller")
		return
	}
	if r.backendCommand == "" {
		fmt.Fprintf(w, "No secret_backend_command set: secrets feature is not enabled")
		return
	}

	t := template.New("secret_info")
	t, err := t.Parse(secretInfoTmpl)
	if err != nil {
		fmt.Fprintf(w, "error parsing secret info template: %s", err)
		return
	}

	t, err = t.Parse(permissionsDetailsTemplate)
	if err != nil {
		fmt.Fprintf(w, "error parsing secret permissions details template: %s", err)
		return
	}

	err = checkRights(r.backendCommand, r.commandAllowGroupExec)

	permissions := "OK, the executable has the correct permissions"
	if err != nil {
		permissions = fmt.Sprintf("error: %s", err)
	}

	details, err := r.getExecutablePermissions()
	info := secretInfo{
		Executable:                   r.backendCommand,
		ExecutablePermissions:        permissions,
		ExecutablePermissionsDetails: details,
		Handles:                      map[string][][]string{},
	}
	if err != nil {
		info.ExecutablePermissionsError = err.Error()
	}

	// we sort handles so the output is consistent and testable
	orderedHandles := []string{}
	for handle := range r.origin {
		orderedHandles = append(orderedHandles, handle)
	}
	sort.Strings(orderedHandles)

	for _, handle := range orderedHandles {
		contexts := r.origin[handle]
		details := [][]string{}
		for _, context := range contexts {
			details = append(details, []string{context.origin, context.yamlPath})
		}
		info.Handles[handle] = details
	}

	err = t.Execute(w, info)
	if err != nil {
		fmt.Fprintf(w, "error rendering secret info: %s", err)
	}
}
