"""
Invoke entrypoint, import here all the tasks we want to make available
"""
from invoke import Collection

from . import agent, benchmarks, docker, dogstatsd, pylauncher

from .go import fmt, lint, vet, deps
from .test import test


# the root namespace
ns = Collection()

# add single tasks to the root
ns.add_task(fmt)
ns.add_task(lint)
ns.add_task(vet)
ns.add_task(test)
ns.add_task(deps)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(benchmarks, name="bench")
ns.add_collection(docker)
ns.add_collection(dogstatsd)
ns.add_collection(pylauncher)
