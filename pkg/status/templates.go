package status

// It might make sense to move these to template files in dist
const forwarder = `
===== Transactions =====

  {{if .TransactionsCreated -}}
  {{- range $key, $value := .TransactionsCreated -}}
  {{$key}}: {{$value}}
  {{end -}}
  {{end}}
`

const checks = `
===== Checks =====

=== Running Checks ===
{{with .RunnerStats -}}
{{- if not .Runs}}
  No checks have run yet
{{end -}}
{{- range .Checks}}
  == {{.CheckName}} ==
    Total Runs: {{.TotalRuns}}
    {{if .LastError -}}
    Error: {{lastErrorMessage .LastError}}
    {{lastErrorTraceback .LastError -}}
    {{- end -}}
{{- end -}}
{{- end}}

{{- with .LoaderStats -}}
{{if .Errors}}
=== Loading Errors ===
  {{ range $checkname, $errors := .Errors }}
  == {{$checkname}} ==
    {{ range $kind, $err := $errors -}}
    {{- if eq $kind "Python Check Loader" -}}
    {{$kind}}: {{ pythonLoaderError $err -}}
    {{- else -}}
    {{$kind}}: {{ doNotEscape $err}}
    {{end -}}
    {{end -}}
  {{end -}}
{{- end}}
{{- end}}
`
