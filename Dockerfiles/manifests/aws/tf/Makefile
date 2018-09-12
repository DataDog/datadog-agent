kubeconfig:
	terraform output kubeconfig > kubeconfig
	export KUBECONFIG=$(PWD)/kubeconfig

config-map-aws-auth:
	terraform output config-map-aws-auth > config-map-aws-auth.yaml
	kubectl apply -f config-map-aws-auth.yaml
	watch -n 1 kubectl get nodes
