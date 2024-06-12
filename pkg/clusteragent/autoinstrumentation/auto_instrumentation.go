package autoinstrumentation

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type language string

const (
	java   language = "java"
	js     language = "js"
	python language = "python"
	dotnet language = "dotnet"
	ruby   language = "ruby"
)

const (
	customLibVersionAnnotationKeyFormat          = "admission.datadoghq.com/%s-lib.version"
	customLibVersionContainerAnnotationKeyFormat = "admission.datadoghq.com/%s.%s-lib.version"

	customImageAnnotationKeyFormat               = "admission.datadoghq.com/%s-lib.custom-image"
	customImageContainerAnnotationKeyFormat      = "admission.datadoghq.com/%s.%s-lib.custom-image"
)

func (l language) customImageAnnotationKey() string {
	return fmt.Sprintf(customImageAnnotationKeyFormat, l)
}

func (l language) customImageContainerAnnotationKey(containerName string) string {
	return fmt.Sprintf(customImageContainerAnnotationKeyFormat, containerName, l)
}

func (l language) customLibVersionAnnotationKey() string {
	return fmt.Sprintf(customLibVersionAnnotationKeyFormat, l)
}

func (l language) customLibVersionContainerAnnotationKey(containerName string) string {
	return fmt.Sprintf(customLibVersionContainerAnnotationKeyFormat, containerName, l)
}

func (l language) String() string {
	return string(l)
}

func (l language) initContainerName() string {
	return fmt.Sprintf("datadog-lib-%s-init", l)
}

func (l language) imageForRegistryAndVersion(registry, version string) string {
	return fmt.Sprintf("%s/dd-lib-%s-init:%s", registry, l, version)
}

func (l language) defaultLanguageInfo(registry string) LanguageInfo {
	return LanguageInfo{
		Image:  l.imageForRegistryAndVersion(registry, "latest"),
		Source: "language-default",
	}
}

type AppendKind struct {
	Override  bool
	Separator string
	NewFirst  bool
}

func (f AppendKind) Join(old, new string) string {
	if old == "" || f.Override {
		return new
	}

	if f.NewFirst {
		return new + f.Separator + old
	}

	return old + f.Separator + new
}

type EnvInjector struct {
	Key    string
	Value  string
	Append AppendKind
}

func (e *EnvInjector) NextValue(prev string) string {
	return e.Append.Join(prev, e.Value)
}

func (e *EnvInjector) StringEnvValue(prev string) string {
	return e.Key + "=" + e.NextValue(prev)
}

func (e *EnvInjector) ContainerEnvVar(prev string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  e.Key,
		Value: e.NextValue(prev),
	}
}

// EnvInjectors is a collection of [EnvInjector] structs
// set by environment variable name.
type EnvInjectors map[string]*EnvInjector

func (e EnvInjectors) duplicate() EnvInjectors {
	d := EnvInjectors{}
	for k, v := range e {
		d[k] = v
	}
	return d
}

func (e EnvInjectors) InjectContainer(c *corev1.Container) error {
	d := e.duplicate()

	for idx, env := range c.Env {
		i, found := d[env.Name]
		if !found {
			continue
		}

		if env.ValueFrom != nil {
			return fmt.Errorf("%q is defined via ValueFrom and cannot be overrriden", env.Name)
		}

		c.Env[idx] = i.ContainerEnvVar(env.Value)
		delete(d, env.Name)
	}

	for _, i := range d {
		c.Env = append(c.Env, i.ContainerEnvVar(""))
	}

	return nil
}

func (e EnvInjectors) InjectEnv(envs []string) ([]string, error) {
	d := e.duplicate()

	for idx, env := range envs {
		name, val, found := strings.Cut(env, "=")
		if !found {
			return nil, errors.New("malformed env")
		}

		i, found := d[name]
		if !found {
			continue
		}

		envs[idx] = i.StringEnvValue(val)
		delete(d, name)
	}

	for _, i := range d {
		envs = append(envs, i.StringEnvValue(""))
	}

	return envs, nil
}

func (e EnvInjectors) collect(is []*EnvInjector) error {
	for _, i := range is {
		if _, alreadyPresent := e[i.Key]; alreadyPresent {
			return fmt.Errorf("key already present %s, they must be unique", i.Key)
		}

		e[i.Key] = i
	}

	return nil
}

func NewEnvInjectors(langs []language) (EnvInjectors, error) {
	i := EnvInjectors{}
	for _, lang := range langs {
		// TODO: check enabled, or we just have a list of supported languages per
		//       version and it's totally fine???
		if err := i.collect(lang.EnvInjectors()); err != nil {
			return nil, err
		}
	}

	return i, nil
}

type InjectsEnvironment interface {
	EnvInjectors() []*EnvInjector
}

func (l language) EnvInjectors() []*EnvInjector {
	switch l {
	case java:
		return []*EnvInjector{
			{
				Key:    "JAVA_TOOL_OPTIONS",
				Value:  "-javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log",
				Append: AppendKind{Separator: " "},
			},
		}
	case js:
		return []*EnvInjector{
			{
				Key:    "NODE_OPTIONS",
				Value:  "--require=/datadog-lib/node_modules/dd-trace/init",
				Append: AppendKind{Separator: " "},
			},
		}
	case python:
		return []*EnvInjector{
			{
				Key:    "PYTHONPATH",
				Value:  "/datadog-lib/",
				Append: AppendKind{Separator: ":", NewFirst: true},
			},
		}
	case dotnet:
		return []*EnvInjector{
			{
				Key:    "CORECLR_ENABLE_PROFILING",
				Value:  "1",
				Append: AppendKind{Override: true},
			},
			{
				Key:    "CORECLR_PROFILER",
				Value:  "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
				Append: AppendKind{Override: true},
			},
			{
				Key:    "CORECLR_PROFILER_PATH",
				Value:  "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
				Append: AppendKind{Override: true},
			},
			{
				Key:    "DD_DOTNET_TRACER_HOME",
				Value:  "/datadog-lib",
				Append: AppendKind{Override: true},
			},
			{
				Key:    "DD_TRACE_LOG_DIRECTORY",
				Value:  "/datadog-lib/logs",
				Append: AppendKind{Override: true},
			},
			{
				Key:    "LD_PRELOAD",
				Value:  "/datadog-lib/continuousprofiler/Datadog.Linux.ApiWrapper.x64.so",
				Append: AppendKind{Separator: ":", NewFirst: true},
			},
		}
	case ruby:
		return []*EnvInjector{
			{
				Key:    "RUBYOPT",
				Value:  "-r/datadog-lib/auto_inject",
				Append: AppendKind{Separator: " "},
			},
		}
	default:
		panic("unknown language: " + string(l))
	}
}
