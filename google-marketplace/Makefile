makefile_dir := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Installs the application CRD on the cluster.
.PHONY: crd/install
crd/install:
	kubectl apply -f $(makefile_dir)crd/app-crd.yaml

# Uninstalls the application CRD from the cluster.
.PHONY: crd/uninstall
crd/uninstall:
	kubectl delete -f $(makefile_dir)crd/app-crd.yaml

# Installs the datadog agent on the cluster.
# Specifically, this renders manifest templates from
# the manifest folder using environment variables defined
# in the README, and apply the resulting manifests.
.PHONY: app/install
app/install:
	cat manifest/* | envsubst > $(makefile_dir)expanded.yaml
	kubectl apply -f $(makefile_dir)expanded.yaml

# Uninstalls the datadog agent from the cluster.
# Also cleans up the rendered manifest.
.PHONY: app/uninstall
app/uninstall:
	kubectl delete -f $(makefile_dir)expanded.yaml
	rm -f $(makefile_dir)expanded.yaml
