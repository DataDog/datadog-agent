plan:
	terraform init && terraform plan -refresh=true -out eks.deploy

apply:
	terraform apply eks.deploy

destroy:
	terraform destroy

kubeconfig:
	terraform output kubeconfig > kubeconfig
	export KUBECONFIG=$(PWD)/kubeconfig

config-map-aws-auth:
	terraform output config-map-aws-auth > config-map-aws-auth.yaml
	kubectl apply -f config-map-aws-auth.yaml
	sleep 30
	kubectl wait --for=condition=Ready nodes --all --timeout=360s
