from invoke.collection import Collection

from tasks.e2e_framework.aws.docker import create_docker, destroy_docker
from tasks.e2e_framework.aws.ecs import create_ecs, destroy_ecs
from tasks.e2e_framework.aws.eks import create_eks, destroy_eks
from tasks.e2e_framework.aws.gensim import deploy_gensim, destroy_gensim, list_gensim_episodes
from tasks.e2e_framework.aws.installer import create_installer_lab, destroy_installer_lab
from tasks.e2e_framework.aws.kind import create_kind, destroy_kind
from tasks.e2e_framework.aws.vm import create_vm, destroy_vm, get_vm_password, rdp_vm, show_vm

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
collection.add_task(deploy_gensim)
collection.add_task(destroy_gensim)
collection.add_task(list_gensim_episodes)
collection.add_task(show_vm)
collection.add_task(get_vm_password)
collection.add_task(rdp_vm)
