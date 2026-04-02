from invoke.collection import Collection

from tasks.e2e_framework.aws.docker import create_docker, destroy_docker
from tasks.e2e_framework.aws.ecs import create_ecs, destroy_ecs
from tasks.e2e_framework.aws.eks import create_eks, destroy_eks
from tasks.e2e_framework.aws.gensim_eks import (
    destroy_gensim_eks,
    status_gensim_eks,
    stop_all_gensim_eks,
    submit_gensim_eks,
    update_manifest_shas_gensim_eks,
)
from tasks.e2e_framework.aws.installer import create_installer_lab, destroy_installer_lab
from tasks.e2e_framework.aws.kind import create_kind, destroy_kind
from tasks.e2e_framework.aws.vm import create_vm, destroy_vm, get_vm_password, rdp_vm, show_vm

collection = Collection()

# aws.eks.gensim.submit / aws.eks.gensim.status / aws.eks.gensim.destroy
# Nested collection structure mirrors the resource hierarchy: cloud → service → scenario.
_gensim_eks_coll = Collection("gensim")
_gensim_eks_coll.add_task(submit_gensim_eks, name="submit")
_gensim_eks_coll.add_task(status_gensim_eks, name="status")
_gensim_eks_coll.add_task(destroy_gensim_eks, name="destroy")
_gensim_eks_coll.add_task(stop_all_gensim_eks, name="stop-all")
_gensim_eks_coll.add_task(update_manifest_shas_gensim_eks, name="update-manifest-shas")
_eks_coll = Collection("eks")
_eks_coll.add_collection(_gensim_eks_coll)
collection.add_collection(_eks_coll)
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
collection.add_task(get_vm_password)
collection.add_task(rdp_vm)
