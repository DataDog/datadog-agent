# type: ignore[reportArgumentType]

from invoke.collection import Collection

from tasks.aws.docker import create_docker, destroy_docker
from tasks.aws.ecs import create_ecs, destroy_ecs
from tasks.aws.eks import create_eks, destroy_eks
from tasks.aws.installer import create_installer_lab, destroy_installer_lab
from tasks.aws.kind import create_kind, destroy_kind
from tasks.aws.vm import create_vm, destroy_vm, show_vm

collection = Collection()
collection.add_task(destroy_vm)
collection.add_task(create_vm)
collection.add_task(create_docker)
collection.add_task(destroy_docker)
collection.add_task(create_ecs)
collection.add_task(destroy_ecs)
collection.add_task(create_eks)
collection.add_task(destroy_eks)
collection.add_task(create_installer_lab)
collection.add_task(destroy_installer_lab)
collection.add_task(create_kind)
collection.add_task(destroy_kind)
collection.add_task(show_vm)
