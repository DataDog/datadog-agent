package datadog

import data.datadog as dd

file_data(file) = d {
        d := {
		"file.group": file.group,
		"file.path": file.path,
		"file.permissions": file.permissions,
		"file.user": file.user,
	}
}

findings[f] {
	f := dd.passed_finding(
		"docker_daemon",
		"the-host_daemon",
		file_data(input.file),
	)
}
