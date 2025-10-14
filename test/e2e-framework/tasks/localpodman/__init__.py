# type: ignore[reportArgumentType]

from invoke.collection import Collection

from tasks.localpodman.vm import create_vm, destroy_vm

collection = Collection()
collection.add_task(destroy_vm)
collection.add_task(create_vm)
