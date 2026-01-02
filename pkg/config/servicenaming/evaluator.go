// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"container/list"
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
)

const (
	// maxCacheSize is the maximum number of compiled programs to cache
	// This prevents unbounded memory growth with dynamic expressions
	maxCacheSize = 1000
)

// cacheEntry represents an entry in the LRU cache
type cacheEntry struct {
	key     string
	program cel.Program
}

// Evaluator handles service discovery evaluation with CEL
type Evaluator struct {
	env          *cel.Env
	programCache map[string]*list.Element // Cache: expr -> list element
	lruList      *list.List               // LRU list (front = most recent)
	cacheMutex   sync.RWMutex             // Thread-safe cache access
}

// NewEvaluator creates a new service discovery evaluator
func NewEvaluator() (*Evaluator, error) {
	env, err := createCELEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	return &Evaluator{
		env:          env,
		programCache: make(map[string]*list.Element),
		lruList:      list.New(),
	}, nil
}

// createCELEnvironment creates the CEL environment with all types
func createCELEnvironment() (*cel.Env, error) {
	env, err := cel.NewEnv(
		// Declare variables as DynType for flexibility with field aliasing
		cel.Variable("process", cel.DynType),
		cel.Variable("container", cel.DynType),
		cel.Variable("pod", cel.DynType),

		// Enable standard CEL string extensions (split, startsWith, etc.)
		ext.Strings(),
	)
	if err != nil {
		return nil, err
	}

	return env, nil
}

// getOrCompileProgram retrieves a cached program or compiles and caches a new one
// Uses LRU eviction: most recently used items stay in cache
func (e *Evaluator) getOrCompileProgram(expr string) (cel.Program, error) {
	// Check cache with read lock
	e.cacheMutex.RLock()
	if elem, ok := e.programCache[expr]; ok {
		entry := elem.Value.(*cacheEntry)
		e.cacheMutex.RUnlock()

		// Move to front (most recently used) with write lock
		e.cacheMutex.Lock()
		e.lruList.MoveToFront(elem)
		e.cacheMutex.Unlock()

		return entry.program, nil
	}
	e.cacheMutex.RUnlock()

	// Not in cache, compile with write lock
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()

	// Double-check in case another goroutine compiled it
	if elem, ok := e.programCache[expr]; ok {
		entry := elem.Value.(*cacheEntry)
		e.lruList.MoveToFront(elem)
		return entry.program, nil
	}

	// Compile the expression
	ast, issues := e.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, err
	}

	// Evict least recently used entry if cache is full
	if len(e.programCache) >= maxCacheSize {
		// Back of list = least recently used
		oldest := e.lruList.Back()
		if oldest != nil {
			oldEntry := oldest.Value.(*cacheEntry)
			delete(e.programCache, oldEntry.key)
			e.lruList.Remove(oldest)
		}
	}

	// Add to cache (front = most recently used)
	entry := &cacheEntry{
		key:     expr,
		program: prg,
	}
	elem := e.lruList.PushFront(entry)
	e.programCache[expr] = elem

	return prg, nil
}

// EvaluateIntegrationConfig evaluates an integration config with the given data
func (e *Evaluator) EvaluateIntegrationConfig(
	config *IntegrationConfig,
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) (*ServiceDiscoveryResult, error) {

	// Check ad_identifiers first (already normalized)
	if len(config.AdIdentifiers) > 0 {
		matched := false
		for _, adID := range config.AdIdentifiers {
			result, err := e.evaluateBooleanExpression(adID, process, container, pod)
			if err != nil {
				// Treat error as false, continue
				continue
			}
			if result {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("no ad_identifier matched")
		}
	}

	// Evaluate service_discovery section
	if config.ServiceDiscovery == nil {
		return nil, fmt.Errorf("service_discovery section is missing")
	}

	result := &ServiceDiscoveryResult{
		MatchedRule: "integration_config",
	}

	// Evaluate service_name
	serviceName, err := e.evaluateStringExpression(config.ServiceDiscovery.ServiceName, process, container, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate service_name: %w", err)
	}
	result.ServiceName = serviceName

	// Evaluate source_name (literal or CEL)
	if config.ServiceDiscovery.SourceName != "" {
		sourceName, err := e.evaluateStringExpressionOrLiteral(config.ServiceDiscovery.SourceName, process, container, pod)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate source_name: %w", err)
		}
		result.SourceName = sourceName
	}

	// Evaluate version (can be literal or CEL)
	if config.ServiceDiscovery.Version != "" {
		version, err := e.evaluateStringExpressionOrLiteral(config.ServiceDiscovery.Version, process, container, pod)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate version: %w", err)
		}
		result.Version = version
	}

	return result, nil
}

// EvaluateAgentConfig evaluates agent-level service discovery config
func (e *Evaluator) EvaluateAgentConfig(
	config *AgentServiceDiscoveryConfig,
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) (*ServiceDiscoveryResult, error) {

	result := &ServiceDiscoveryResult{}

	// Evaluate service_definitions in order (first match wins)
	for i, def := range config.ServiceDefinitions {
		// Evaluate query
		matched, err := e.evaluateBooleanExpression(def.Query, process, container, pod)
		if err != nil {
			// Query error → treat as false, continue
			continue
		}
		if !matched {
			continue
		}

		// Evaluate value
		value, err := e.evaluateStringExpression(def.Value, process, container, pod)
		if err != nil || value == "" {
			// Value error or empty → continue to next rule
			continue
		}

		// Success
		result.ServiceName = value
		result.MatchedRule = fmt.Sprintf("service_definition[%d]", i)
		break
	}

	// Evaluate source_definition
	if config.SourceDefinition != "" {
		sourceName, err := e.evaluateStringExpressionOrLiteral(config.SourceDefinition, process, container, pod)
		if err == nil {
			result.SourceName = sourceName
		}
	}

	// Evaluate version_definition (can be literal or CEL)
	if config.VersionDefinition != "" {
		version, err := e.evaluateStringExpressionOrLiteral(config.VersionDefinition, process, container, pod)
		if err == nil {
			result.Version = version
		}
	}

	return result, nil
}

// evaluateBooleanExpression evaluates a CEL expression that returns boolean
func (e *Evaluator) evaluateBooleanExpression(
	expr string,
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) (bool, error) {
	// Get or compile the program (with caching)
	prg, err := e.getOrCompileProgram(expr)
	if err != nil {
		return false, err
	}

	vars := e.buildVars(process, container, pod)
	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, err
	}

	boolVal, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression did not return boolean")
	}

	return boolVal, nil
}

// evaluateStringExpression evaluates a CEL expression that returns string
func (e *Evaluator) evaluateStringExpression(
	expr string,
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) (string, error) {
	// Get or compile the program (with caching)
	prg, err := e.getOrCompileProgram(expr)
	if err != nil {
		return "", err
	}

	vars := e.buildVars(process, container, pod)
	out, _, err := prg.Eval(vars)
	if err != nil {
		return "", err
	}

	strVal, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("expression did not return string")
	}

	return strVal, nil
}

// evaluateStringExpressionOrLiteral evaluates expression or returns literal
func (e *Evaluator) evaluateStringExpressionOrLiteral(
	expr string,
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) (string, error) {
	// Try to get or compile as CEL (using cache)
	prg, err := e.getOrCompileProgram(expr)
	if err != nil {
		// Not valid CEL → treat as literal string
		return expr, nil
	}

	// Valid CEL → evaluate using cached program
	vars := e.buildVars(process, container, pod)
	out, _, err := prg.Eval(vars)
	if err != nil {
		return "", err
	}

	strVal, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("expression did not return string")
	}

	return strVal, nil
}

// buildVars creates the variable map for CEL evaluation
func (e *Evaluator) buildVars(
	process *ProcessCEL,
	container *ContainerCEL,
	pod *PodCEL,
) map[string]interface{} {
	vars := make(map[string]interface{})

	if process != nil {
		vars["process"] = normalizeProcess(process)
	}
	if container != nil {
		vars["container"] = normalizeContainer(container)
	}
	if pod != nil {
		vars["pod"] = normalizePod(pod)
	}

	return vars
}

// normalizeProcess creates a normalized map with field aliases
func normalizeProcess(p *ProcessCEL) map[string]interface{} {
	return map[string]interface{}{
		"cmd":    p.Cmd,
		"binary": normalizeBinary(&p.Binary),
		"ports":  p.Ports,
		"user":   p.User,
	}
}

// normalizeBinary creates a normalized map
func normalizeBinary(b *BinaryCEL) map[string]interface{} {
	return map[string]interface{}{
		"name":  b.Name,
		"user":  b.User,
		"group": b.Group,
	}
}

// normalizeContainer creates a normalized map with field aliases and cross-references
func normalizeContainer(c *ContainerCEL) map[string]interface{} {
	container := map[string]interface{}{
		"name":  c.Name,
		"image": normalizeImage(&c.Image),
		"pod":   normalizePod(&c.Pod),
	}
	return container
}

// normalizeImage creates a normalized map with shortname/short_name aliasing
func normalizeImage(img *ImageCEL) map[string]interface{} {
	return map[string]interface{}{
		"name":       img.Name,
		"shortname":  img.ShortName,
		"short_name": img.ShortName, // Alias
		"tag":        img.Tag,
	}
}

// normalizePod creates a normalized map with ownerref aliasing
func normalizePod(p *PodCEL) map[string]interface{} {
	pod := map[string]interface{}{
		"name":      p.Name,
		"namespace": p.Namespace,
		"metadata":  normalizeMetadata(&p.Metadata),
		// Support both flat and nested ownerref access
		"ownerrefname": p.OwnerRefName,
		"ownerrefkind": p.OwnerRefKind,
		"ownerref": map[string]interface{}{
			"name": p.OwnerRefName,
			"kind": p.OwnerRefKind,
		},
	}
	return pod
}

// normalizeMetadata creates a normalized map
func normalizeMetadata(m *MetadataCEL) map[string]interface{} {
	return map[string]interface{}{
		"labels": m.Labels,
	}
}

// ResolveSDPlaceholders replaces %%SD_*%% placeholders with actual values
func ResolveSDPlaceholders(template string, result *ServiceDiscoveryResult, container *ContainerCEL) string {
	replacements := map[string]string{
		"%%SD_service_name%%": result.ServiceName,
		"%%SD_source_name%%":  result.SourceName,
		"%%SD_version%%":      result.Version,
	}

	// Add container field placeholders
	if container != nil {
		replacements["%%SD_container.image.short_name%%"] = container.Image.ShortName
		replacements["%%SD_container.image.shortname%%"] = container.Image.ShortName
		replacements["%%SD_container.image.tag%%"] = container.Image.Tag
		replacements["%%SD_container.image.name%%"] = container.Image.Name
		replacements["%%SD_container.name%%"] = container.Name
	}

	output := template
	for placeholder, value := range replacements {
		output = strings.ReplaceAll(output, placeholder, value)
	}

	return output
}

// validateCELBooleanExpression validates that an expression compiles and returns boolean
func validateCELBooleanExpression(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// Check return type
	if ast.OutputType() != cel.BoolType {
		return fmt.Errorf("expression must return boolean, got %v", ast.OutputType())
	}

	return nil
}

// validateCELStringExpression validates that an expression compiles and returns string
func validateCELStringExpression(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// For DynType, we can't check output type statically
	// Accept dyn or string
	outType := ast.OutputType()
	if outType != cel.StringType && outType != types.DynType {
		return fmt.Errorf("expression must return string, got %v", outType)
	}

	return nil
}

// validateCELStringExpressionOrLiteral validates expression or accepts literal
func validateCELStringExpressionOrLiteral(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	_, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		// Not valid CEL → treat as literal, which is OK
		return nil
	}

	// Valid CEL → no further validation needed
	return nil
}
