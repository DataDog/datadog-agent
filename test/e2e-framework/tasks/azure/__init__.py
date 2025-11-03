# type: ignore[reportArgumentType]

from invoke.collection import Collection

from tasks.azure.aks import create_aks, destroy_aks
from tasks.azure.vm import create_vm, destroy_vm

collection = Collection()
collection.add_task(destroy_vm)
collection.add_task(create_vm)
collection.add_task(create_aks)
collection.add_task(destroy_aks)
