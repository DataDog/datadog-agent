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
        f := dd.failing_finding(
                "kubernetes_node",
                "kube_system_uuid_kubernetes_node",
                file_data(input.file),
        )
}
