name 'datadog-agent-env-check'

description "Execute pip check on the python environment of the agent to make sure everything is compatible"

# Run the check after all the definitions touching the python environment of the agent.
dependency "pip"
dependency "datadog-agent"
dependency "stackstate-agent-integrations"

build do
    # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
    pip "check"
end
