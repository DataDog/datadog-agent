from __future__ import annotations

import sys
from typing import TYPE_CHECKING, Callable, Iterable, List

from tasks.kernel_matrix_testing.tool import info

if TYPE_CHECKING:
    import libvirt

    from tasks.kernel_matrix_testing.types import TNamed
else:
    try:
        import libvirt
    except ImportError:
        libvirt = None


def resource_in_stack(stack: str, resource: str) -> bool:
    return f"-{stack}" in resource


def get_resources_in_stack(stack: str, list_fn: Callable[[], Iterable[TNamed]]) -> List[TNamed]:
    resources = list_fn()
    stack_resources = list()
    for resource in resources:
        if resource_in_stack(stack, resource.name()):
            stack_resources.append(resource)

    return stack_resources


def delete_domains(conn: 'libvirt.virConnect', stack: str):
    domains = get_resources_in_stack(stack, conn.listAllDomains)
    info(f"[*] {len(domains)} VMs running in stack {stack}")

    for domain in domains:
        name = domain.name()
        if domain.isActive():
            domain.destroy()

        undefine_flags = 0
        if sys.platform == "darwin":
            undefine_flags |= libvirt.VIR_DOMAIN_UNDEFINE_NVRAM

        domain.undefineFlags(undefine_flags)

        info(f"[+] VM {name} deleted")


def getAllStackVolumesFn(conn: 'libvirt.virConnect', stack: str):
    def getAllStackVolumes() -> List['libvirt.virStorageVol']:
        pools = get_resources_in_stack(stack, conn.listAllStoragePools)

        volumes: List['libvirt.virStorageVol'] = list()
        for pool in pools:
            if not pool.isActive():
                continue
            volumes += pool.listAllVolumes()

        return volumes

    return getAllStackVolumes


def delete_volumes(conn: 'libvirt.virConnect', stack: str):
    volumes = get_resources_in_stack(stack, getAllStackVolumesFn(conn, stack))
    info(f"[*] {len(volumes)} storage volumes running in stack {stack}")

    for volume in volumes:
        name = volume.name()
        volume.delete()
        #        volume.undefine()
        info(f"[+] Storage volume {name} deleted")


def delete_pools(conn: 'libvirt.virConnect', stack: str):
    pools = get_resources_in_stack(stack, conn.listAllStoragePools)
    info(f"[*] {len(pools)} storage pools running in stack {stack}")

    for pool in pools:
        name = pool.name()
        if pool.isActive():
            pool.destroy()
        pool.undefine()
        info(f"[+] Storage pool {name} deleted")


def delete_networks(conn: 'libvirt.virConnect', stack: str):
    networks = get_resources_in_stack(stack, conn.listAllNetworks)
    info(f"[*] {len(networks)} networks running in stack {stack}")

    for network in networks:
        name = network.name()
        if network.isActive():
            network.destroy()
        network.undefine()
        info(f"[+] Network {name} deleted")


def pause_domains(conn: 'libvirt.virConnect', stack: str):
    domains = get_resources_in_stack(stack, conn.listAllDomains)
    info(f"[*] {len(domains)} VMs running in stack {stack}")

    for domain in domains:
        name = domain.name()
        if domain.isActive():
            domain.destroy()
        info(f"[+] VM {name} is paused")


def resume_network(conn: 'libvirt.virConnect', stack: str):
    networks = get_resources_in_stack(stack, conn.listAllNetworks)
    info(f"[*] {len(networks)} networks running in stack {stack}")

    for network in networks:
        name = network.name()
        if not network.isActive():
            network.create()
        info(f"[+] Network {name} resumed")


def resume_domains(conn: 'libvirt.virConnect', stack: str):
    domains = get_resources_in_stack(stack, conn.listAllDomains)
    info(f"[*] {len(domains)} VMs running in stack {stack}")

    resume_network(conn, stack)

    for domain in domains:
        name = domain.name()
        if not domain.isActive():
            domain.create()
        info(f"[+] VM {name} is resumed")
