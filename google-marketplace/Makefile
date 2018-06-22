makefile_dir := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Installs the application CRD on the cluster.
.PHONY: crd/install
crd/install:
	kubectl apply -f $(makefile_dir)crd/app-crd.yaml

# Uninstalls the application CRD from the cluster.
.PHONY: crd/uninstall
crd/uninstall:
	kubectl delete -f $(makefile_dir)crd/app-crd.yaml

