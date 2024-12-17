module github.com/DataDog/datadog-agent/comp/api/authtoken/impl-none

replace github.com/DataDog/datadog-agent/comp/api/authtoken/def => ../def

go 1.22.0

require github.com/DataDog/datadog-agent/comp/api/authtoken/def v0.0.0-00010101000000-000000000000
