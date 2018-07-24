makefile_dir := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Installs the application CRD on the cluster.
.PHONY: crd/install
crd/install:
	kubectl apply -f $(makefile_dir)crd/app-crd.yaml

# Uninstalls the application CRD from the cluster.
.PHONY: crd/uninstall
crd/uninstall:
	kubectl delete -f $(makefile_dir)crd/app-crd.yaml

# Installs the RBAC objects (ServiceAccount, ClusterRole, ClusterRoleBinding) on the cluster.
.PHONY: rbac/install
rbac/install:
	cat cmdline_rbac/datadog-agent-rbac.yaml.template | envsubst > $(makefile_dir)rbac-expanded.yaml
	kubectl apply -f $(makefile_dir)rbac-expanded.yaml

# Uninstalls the Datadog-related RBAC objects from the cluster.
.PHONY: rbac/uninstall
rbac/uninstall:
	kubectl delete -f $(makefile_dir)rbac-expanded.yaml
	rm -f $(makefile_dir)rbac-expanded.yaml 

# Installs the datadog agent on the cluster.
# Specifically, this renders manifest templates for the
# DaemonSet, Secret, and Application objects from
# the manifest folder using environment variables defined
# in the README, and apply the resulting manifests.
# Make sure RBAC objects are already created before calling this.
.PHONY: app/install
app/install:
	cat manifest/datadog-agent-daemonset.yaml.template manifest/application.yaml.template | envsubst > $(makefile_dir)app-expanded.yaml
	kubectl apply -f $(makefile_dir)app-expanded.yaml

# Uninstalls the datadog agent from the cluster.
# Also cleans up the rendered manifest.
.PHONY: app/uninstall
app/uninstall:
	kubectl delete -f $(makefile_dir)app-expanded.yaml
	rm -f $(makefile_dir)app-expanded.yaml
