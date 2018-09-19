# Kubernetes Cluster Setup

### Variables

To create a cluster you  need to define the following environment variables:

```
export TF_VAR_AWS_SECRET_ACCESS_KEY=...
export TF_VAR_AWS_ACCESS_KEY_ID=...
export TF_VAR_CLUSTER_NAME=...
```

You should not need to change other default variables, but you never know :)

```
export TF_VAR_AWS_REGION= (default to us-east-1)
export TF_VAR_SSH_KEY_PAIR= (default to EKS)
```

### Provision

If is the first time you provision with those script `terraform init`.

If you want to check what changes Terraform will apply you can see it with `terraform plan`.

Apply the changes `terraform apply`.

The provision takes around 20mins.


### Configure kubectl

To allow kubectl to talk to your cluster `make kubeconfig` and follow the suggestion to export kube config path.

Make sure nodes can register `make config-map-aws-auth` and wait for nodes to appear.

Your are now ready.

### Setup on an already provisioned Cluster

Get the kubectl command and aws-iam-authenticator command
Get the kubeconfig file form someone and put it somewhere
Get the aws credentials and put them in ~/.aws/credentials

> export KUBECONFIG=<path_to_kubeconfig>

### Access the dashboard

> kubectl proxy --port=8001
> browse to http://localhost:8001/api/v1/namespaces/kube-system/services/https:kubernetes-dashboard:/proxy/#!/overview?namespace=default
