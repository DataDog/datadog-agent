{{ define "option" }}
  {{- if and (not .Undocumented) (not .NoEnvvar) }}
  - `{{ .Envvar }}`: {{ if .EnvvarDesc }}{{ .EnvvarDesc }}{{ else }}{{ .Description }}{{ end }}
  {{- end -}}
  {{- range .SubOptions}}
    {{- template "option" . }}
  {{- end }}
{{- end }}
# Available environment variable bindings

This document lists all available environment variable bindings. Please refer to [the example datadog.yaml](todo.link) for longer descriptions of each option.
{{ range . }}
## {{ .Name }}

{{ .Description | prefix "" }}
  {{ range .Options }}
    {{- template "option" . }}
  {{- end }}
{{ end -}}
