package helpers

import data.datadog as dd
import future.keywords.if

has_key(o, k) {
	_ := o[k]
}

resource_type = rt {
	rt := input.constants.resource_type
}

resource_id = rid {
	input.constants.resource_type == "docker_daemon"
	rid := docker_daemon_resource_id
}

resource_id = rid {
	input.constants.resource_type == "kubernetes_worker_node"
	rid := kubernetes_worker_node_resource_id
}

resource_id = rid {
	input.constants.resource_type == "kubernetes_cluster"
	rid := kubernetes_cluster_resource_id
}

resource_id = rid {
	input.constants.resource_type == "kubernetes_master_node"
	rid := kubernetes_master_node_resource_id
}

resource_id = rid {
	input.constants.resource_type == "docker_container"
	rid := docker_daemon_resource_id
}

kubernetes_cluster_resource_id = id {
	id := sprintf("%s_kubernetes_cluster", [input.context.kubernetes_cluster])
}

kubernetes_master_node_resource_id = id {
	id := sprintf("%s_kubernetes_master_node", [input.context.hostname])
}

kubernetes_worker_node_resource_id = id {
	id := sprintf("%s_kubernetes_worker_node", [input.context.hostname])
}

kubernetes_resource_names(serviceaccounts) = x {
	x := [{"name": name, "namespace": namespace} |
		name := serviceaccounts.name
		namespace := serviceaccounts.namespace
	]
}

docker_container_resource_id(c) = id {
	id := sprintf("%s_%s", [input.context.hostname, cast_string(c.id)])
}

docker_image_resource_id(img) = id {
	hash := split(cast_string(img.id), ":")[1]
	id := sprintf("%s_%s", [input.context.hostname, hash])
}

docker_daemon_resource_id = id {
	id := sprintf("%s_daemon", [input.context.hostname])
}

docker_network_resource_id(n) = id {
	id := sprintf("%s_%s", [input.context.hostname, cast_string(n.id)])
}

docker_container_data(c) = d {
	d := {
		"container.id": c.id,
		"container.image": c.inspect.Config.Image,
		"container.name": c.name,
	}
}

docker_image_data(img) = d {
	d := {
		"image.id": img.id,
		"image.tags": img.tags,
	}
}

docker_network_data(network) = d {
	d := {"network.name": network.name}
}

process_data(p) = d {
	d := {
		"process.name": p.name,
		"process.exe": p.exe,
		"process.cmdLine": p.cmdLine,
	}
}

file_data(file) = d {
	d := {
		"file.group": file.group,
		"file.path": file.path,
		"file.permissions": file.permissions,
		"file.user": file.user,
		"file.content": file.content,
	}
}

group_data(group) = d {
	d := {
		"group.id": group.id,
		"group.name": group.name,
		"group.users": group.users,
	}
}

audit_data(audit) = d {
	d := {
		"audit.enabled": audit.enabled,
		"audit.path": audit.path,
		"audit.permissions": audit.permissions,
	}
}

file_process_flag(name) := resource if {
	process := input.process
	process.flags[name]
	resource := json.patch(input.context.input.file.file, [{"op": "replace", "path": "/path", "value": process.flags[name]}]) 
}
