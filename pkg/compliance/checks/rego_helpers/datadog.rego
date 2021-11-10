package datadog

raw_finding(status, resource_type, resource_id, event_data) = f {
	f := {
		"status": status,
		"resource_type": resource_type,
		"resource_id": resource_id,
		"data": event_data,
	}
}

kubernetes_cluster_resource_id = id {
	id := sprintf("%s_kubernetes_cluster", [input.context.kubernetes_cluster])
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

kubernetes_master_node_resource_id = id {
	id := sprintf("%s_kubernetes_master_node", [input.context.hostname])
}

kubernetes_worker_node_resource_id = id {
	id := sprintf("%s_kubernetes_worker_node", [input.context.hostname])
}

docker_network_resource_id(n) = id {
	id := sprintf("%s_%s", [input.context.hostname, cast_string(n.id)])
}

passed_finding(resource_type, resource_id, event_data) = f {
	f := raw_finding("passed", resource_type, resource_id, event_data)
}

failing_finding(resource_type, resource_id, event_data) = f {
	f := raw_finding("failing", resource_type, resource_id, event_data)
}

error_finding(resource_type, resource_id, error_msg) = f {
	f := raw_finding("error", resource_type, resource_id, {
		"error": error_msg
	})
}

docker_container_data(c) = d {
	d := {
		"container.id": c.id,
		"container.image": c.image,
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
	d := {
		"network.name": network.name,
	}
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
