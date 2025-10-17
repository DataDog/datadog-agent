# type: ignore[reportArgumentType]

from invoke.collection import Collection

import tasks.ci as ci
import tasks.setup as setup
import tasks.test as test
from tasks import aws, azure, gcp, localpodman, platforms
from tasks.aks import create_aks, destroy_aks
from tasks.deploy import check_s3_image_exists
from tasks.docker import create_docker, destroy_docker
from tasks.ecs import create_ecs, destroy_ecs
from tasks.eks import create_eks, destroy_eks
from tasks.installer import create_installer_lab, destroy_installer_lab
from tasks.kind import create_kind, destroy_kind
from tasks.pipeline import retry_job
from tasks.vm import create_vm, destroy_vm, get_vm_password, rdp_vm

ns = Collection()

ns.add_task(check_s3_image_exists)
ns.add_task(create_aks)
ns.add_task(create_docker)
ns.add_task(create_ecs)
ns.add_task(create_eks)
ns.add_task(create_installer_lab)
ns.add_task(create_kind)
ns.add_task(create_vm)
ns.add_task(destroy_aks)
ns.add_task(destroy_docker)
ns.add_task(destroy_ecs)
ns.add_task(destroy_eks)
ns.add_task(destroy_installer_lab)
ns.add_task(destroy_kind)
ns.add_task(destroy_vm)
ns.add_task(get_vm_password)
ns.add_task(rdp_vm)
ns.add_task(retry_job)

ns.add_collection(platforms, "platforms")
ns.add_collection(aws.collection, "aws")
ns.add_collection(azure.collection, "az")
ns.add_collection(gcp.collection, "gcp")
ns.add_collection(localpodman.collection, "localpodman")
ns.add_collection(ci)
ns.add_collection(setup)
ns.add_collection(test)
