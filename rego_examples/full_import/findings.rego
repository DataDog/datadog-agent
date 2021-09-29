package demo

import data.datadog as dd
import data.demo_import as di

findings[f] {
	c := input.containers[_]
	di.valid_container(c)

	f := dd.passed_finding(
		"docker_container",
		dd.docker_container_resource_id(c),
		dd.docker_container_data(c)
	)
}

findings[f] {
	c := input.containers[_]
	not di.valid_container(c)

	f := dd.failing_finding(
		"docker_container",
		dd.docker_container_resource_id(c),
		dd.docker_container_data(c)
	)
}
