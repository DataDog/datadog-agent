0. AWS key pair in environment vars
1. Checked variables.tf for customized things, if any
2. terraform init
3. terraform plan
4. terraform apply
wait ~ 15-22 mins for Apply complete! 
5. make kubelet-config
follow hint to export config path for kubectl
6. Make sure nodes can register
make config-map-aws-auth

wait for list of nodes to appear

7. kubectl create -f stackstate-serviceaccount.yaml

8.
