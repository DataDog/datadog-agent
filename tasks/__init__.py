"""
Invoke entrypoint, import here all the tasks we want to make available
"""
import os
from invoke import Collection

from . import agent, benchmarks, docker, dogstatsd, pylauncher

from .go import fmt, lint, vet, deps, reset
from .test import test, integration_tests


# the root namespace
ns = Collection()

# add single tasks to the root
ns.add_task(fmt)
ns.add_task(lint)
ns.add_task(vet)
ns.add_task(test)
ns.add_task(integration_tests)
ns.add_task(deps)
ns.add_task(reset)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(benchmarks, name="bench")
ns.add_collection(docker)
ns.add_collection(dogstatsd)
ns.add_collection(pylauncher)

ns.configure({
    'run': {
        # workaround waiting for a fix being merged on Invoke,
        # see https://github.com/pyinvoke/invoke/pull/407
        'shell': os.environ.get('COMSPEC', os.environ.get('SHELL'))
    }
})
