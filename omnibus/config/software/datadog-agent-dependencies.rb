name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

if with_python_runtime? "2"
  dependency 'pylint2'
  dependency 'datadog-agent-integrations-py2-dependencies'
end

if with_python_runtime? "3"
  dependency 'datadog-agent-integrations-py3-dependencies'
end
