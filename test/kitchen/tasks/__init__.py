"""
Invoke entrypoint, import here all the tasks we want to make available
"""

from invoke import Collection

from . import kitchen

ns = Collection()

ns.add_collection(kitchen)
