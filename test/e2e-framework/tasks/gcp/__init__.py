# type: ignore[reportArgumentType]

from invoke.collection import Collection

from tasks.gcp.gke import create_gke, destroy_gke
from tasks.gcp.openshift import create_openshift, destroy_openshift
from tasks.gcp.vm import create_vm, destroy_vm

collection = Collection()
collection.add_task(destroy_vm)
collection.add_task(create_vm)
collection.add_task(create_gke)
collection.add_task(destroy_gke)
collection.add_task(create_openshift)
collection.add_task(destroy_openshift)
