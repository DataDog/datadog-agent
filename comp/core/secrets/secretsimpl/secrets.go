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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.uber.org/fx"
	"golang.org/x/exp/maps"
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

type secretContext struct {
	// origin is the configuration name where a handle was found
	origin string
	// path is the key associated with the secret in the YAML configuration,
	// represented as a list of field names
	// Example: in this yaml: '{"service": {"token": "ENC[my_token]"}}', ['service', 'token'] is the path and 'my_token' is the handle.
	path []string
}

type handleToContext map[string][]secretContext

type secretResolver struct {
	enabled bool
	lock    sync.Mutex
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
	// refresh secrets at a regular interval
	refreshInterval time.Duration
	ticker          *time.Ticker
	// subscriptions want to be notified about changes to the secrets
	subscriptions []secrets.SecretChangeCallback

	// can be overridden for testing purposes
	commandHookFunc func(string) ([]byte, error)
	fetchHookFunc   func([]string) (map[string]string, error)
	scrubHookFunc   func([]string)
}

var _ secrets.Component = (*secretResolver)(nil)

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

func (r *secretResolver) registerSecretOrigin(handle string, origin string, path []string) {
	for _, info := range r.origin[handle] {
		if info.origin == origin && slices.Equal(info.path, path) {
			// The secret was used twice in the same configuration under the same key: nothing to do
			return
		}
	}

	if len(path) != 0 {
		lastElem := path[len(path)-1:]
		// work around a bug in the scrubber: if the last element looks like an
		// index into a slice, remove it and use the element before
		if _, err := strconv.Atoi(lastElem[0]); err == nil && len(path) >= 2 {
			lastElem = path[len(path)-2 : len(path)-1]
		}
		if r.scrubHookFunc != nil {
			// hook used only for tests
			r.scrubHookFunc(lastElem)
		} else {
			scrubber.AddStrippedKeys(lastElem)
		}
	}

	// clone the path to take ownership of it, otherwise callers may
	// modify the original object and corrupt data in the origin map
	path = slices.Clone(path)

	r.origin[handle] = append(
		r.origin[handle],
		secretContext{
			origin: origin,
			path:   path,
		})
}

// Configure initializes the executable command and other options of the secrets component
func (r *secretResolver) Configure(command string, arguments []string, timeout, maxSize, refreshInterval int, groupExecPerm, removeLinebreak bool) {
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
	r.refreshInterval = time.Duration(refreshInterval) * time.Second
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

func (r *secretResolver) startRefreshRoutine() {
	if r.ticker != nil || r.refreshInterval == 0 {
		return
	}
	r.ticker = time.NewTicker(r.refreshInterval)
	go func() {
		for {
			<-r.ticker.C
			if _, err := r.Refresh(); err != nil {
				log.Info(err)
			}
		}
	}()
}

// SubscribeToChanges adds this callback to the list that get notified when secrets are resolved or refreshed
func (r *secretResolver) SubscribeToChanges(cb secrets.SecretChangeCallback) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.startRefreshRoutine()
	r.subscriptions = append(r.subscriptions, cb)
}

// Resolve replaces all encoded secrets in data by executing "secret_backend_command" once if all secrets aren't
// present in the cache.
func (r *secretResolver) Resolve(data []byte, origin string) ([]byte, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

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
	foundSecrets := map[string]struct{}{}

	w := &walker{
		resolver: func(path []string, value string) (string, error) {
			if ok, handle := isEnc(value); ok {
				// Check if we already know this secret
				if secretValue, ok := r.cache[handle]; ok {
					log.Debugf("Secret '%s' was retrieved from cache", handle)
					// keep track of place where a handle was found
					r.registerSecretOrigin(handle, origin, path)
					// notify subscriptions
					for _, sub := range r.subscriptions {
						sub(handle, origin, path, secretValue, secretValue)
					}
					foundSecrets[handle] = struct{}{}
					return secretValue, nil
				}
				// only add handle to newHandles list if it wasn't seen yet
				if _, ok := foundSecrets[handle]; !ok {
					newHandles = append(newHandles, handle)
				}
				foundSecrets[handle] = struct{}{}
				return value, nil
			}
			return value, nil
		},
	}

	if err := w.walk(&config); err != nil {
		return nil, err
	}

	// the configuration does not contain any secrets
	if len(foundSecrets) == 0 {
		return data, nil
	}

	// check if any new secrets need to be fetch
	if len(newHandles) != 0 {
		var secretResponse map[string]string
		var err error
		if r.fetchHookFunc != nil {
			// hook used only for tests
			secretResponse, err = r.fetchHookFunc(newHandles)
		} else {
			secretResponse, err = r.fetchSecret(newHandles)
		}
		if err != nil {
			return nil, err
		}

		w.resolver = func(path []string, value string) (string, error) {
			if ok, handle := isEnc(value); ok {
				if secretValue, ok := secretResponse[handle]; ok {
					log.Debugf("Secret '%s' was successfully resolved", handle)
					// keep track of place where a handle was found
					r.registerSecretOrigin(handle, origin, path)
					return secretValue, nil
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

		// for Resolving secrets, always send notifications
		r.processSecretResponse(secretResponse, false)
	}

	finalConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not Marshal config after replacing encrypted secrets: %s", err)
	}
	return finalConfig, nil
}

// allowlistPaths restricts what config settings may be updated
// tests can override this to exercise functionality: by setting this to nil, allow all settings
// NOTE: Related feature to `authorizedConfigPathsCore` in `comp/api/api/apiimpl/internal/config/endpoint.go`
var allowlistPaths = map[string]struct{}{"api_key": {}}

// matchesAllowlist returns whether the handle is allowed, by matching all setting paths that
// handle appears at against the allowlist
func (r *secretResolver) matchesAllowlist(handle string) bool {
	// if allowlist is disabled, consider every handle a match
	if allowlistPaths == nil {
		return true
	}
	for _, secretCtx := range r.origin[handle] {
		if _, ok := allowlistPaths[strings.Join(secretCtx.path, "/")]; ok {
			return true
		}
	}
	// the handle does not appear for a setting that is in the allowlist
	return false
}

func (r *secretResolver) processSecretResponse(secretResponse map[string]string, useAllowlist bool) secretRefreshInfo {
	var handleInfoList []handleInfo

	// notify subscriptions about the changes to secrets
	for handle, secretValue := range secretResponse {
		oldValue := r.cache[handle]
		// if value hasn't changed, don't send notifications
		if oldValue == secretValue {
			continue
		}

		// if allowlist is enabled and the config setting path is not contained in it, skip it
		if useAllowlist && !r.matchesAllowlist(handle) {
			continue
		}

		places := make([]handlePlace, 0, len(r.origin[handle]))
		for _, secretCtx := range r.origin[handle] {
			for _, sub := range r.subscriptions {
				secretPath := strings.Join(secretCtx.path, "/")
				// only update setting paths that match the allowlist
				if useAllowlist && allowlistPaths != nil {
					if _, ok := allowlistPaths[secretPath]; !ok {
						continue
					}
				}
				// notify subscribers that secret has changed
				sub(handle, secretCtx.origin, secretCtx.path, oldValue, secretValue)
				places = append(places, handlePlace{Context: secretCtx.origin, Path: secretPath})
			}
		}
		handleInfoList = append(handleInfoList, handleInfo{Name: handle, Places: places})
	}
	// add results to the cache
	for handle, secretValue := range secretResponse {
		r.cache[handle] = secretValue
	}
	// return info about the handles sorted by their name
	sort.Slice(handleInfoList, func(i, j int) bool {
		return handleInfoList[i].Name < handleInfoList[j].Name
	})
	return secretRefreshInfo{Handles: handleInfoList}
}

// Refresh the secrets after they have been Resolved by fetching them from the backend again
func (r *secretResolver) Refresh() (string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// get handles from the cache that match the allowlist
	newHandles := maps.Keys(r.cache)
	if allowlistPaths != nil {
		filteredHandles := make([]string, 0, len(newHandles))
		for _, handle := range newHandles {
			if r.matchesAllowlist(handle) {
				filteredHandles = append(filteredHandles, handle)
			}
		}
		newHandles = filteredHandles
	}
	if len(newHandles) == 0 {
		return "", nil
	}

	log.Infof("Refreshing secrets for %d handles", len(newHandles))

	var secretResponse map[string]string
	var err error
	if r.fetchHookFunc != nil {
		// hook used only for tests
		secretResponse, err = r.fetchHookFunc(newHandles)
	} else {
		secretResponse, err = r.fetchSecret(newHandles)
	}
	if err != nil {
		return "", err
	}

	// when Refreshing secrets, only update what the allowlist allows by passing `true`
	refreshResult := r.processSecretResponse(secretResponse, true)

	// render a report
	t := template.New("secret_refresh")
	t, err = t.Parse(secretRefreshTmpl)
	if err != nil {
		return "", err
	}
	b := new(strings.Builder)
	err = t.Execute(b, refreshResult)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

type secretInfo struct {
	Executable                   string
	ExecutablePermissions        string
	ExecutablePermissionsDetails interface{}
	ExecutablePermissionsError   string
	Handles                      map[string][][]string
}

type secretRefreshInfo struct {
	Handles []handleInfo
}

type handleInfo struct {
	Name   string
	Places []handlePlace
}

type handlePlace struct {
	Context string
	Path    string
}

//go:embed info.tmpl
var secretInfoTmpl string

//go:embed refresh.tmpl
var secretRefreshTmpl string

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
			details = append(details, []string{context.origin, strings.Join(context.path, "/")})
		}
		info.Handles[handle] = details
	}

	err = t.Execute(w, info)
	if err != nil {
		fmt.Fprintf(w, "error rendering secret info: %s", err)
	}
}
