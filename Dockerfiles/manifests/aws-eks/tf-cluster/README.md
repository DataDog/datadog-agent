# Kubernetes Cluster Setup

Important: setup follows https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html as of commit date.

In order to start with cluster setup, you will need:

- Terraform
- AWS credentials with necessary rights
- AWS authenticator for EKS, called heptio authenticator:
  ```
  curl -o <PATH>/heptio-authenticator-aws https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-06-05/bin/linux/amd64/heptio-authenticator-aws
  curl -o <PATH>/heptio-authenticator-aws.md5 https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-06-05/bin/linux/amd64/heptio-authenticator-aws.md5
  chmod +x <PATH>/heptio-authenticator-aws
  ```

## Variables

Create an ssh key pair that will be used for login in to the EC2 instances:

`ssh-keygen -f eks_rsa`

Set the following environment variables:

```
export AWS_ACCESS_KEY_ID=
export AWS_SECRET_ACCESS_KEY=
export TF_VAR_AWS_SECRET_ACCESS_KEY=...
export TF_VAR_AWS_ACCESS_KEY_ID=...
export TF_VAR_AWS_REGION=us-east-1
export TF_VAR_CLUSTER_NAME=...
```

## Deploy

### Plan and apply

First `make plan` to check what changes Terraform will apply, then deploy the changes with `make apply`.

The operation takes around 20mins.

### Output

Output are:

- `kubeconfig` file produced by `make kubeconfig`

   You can `export KUBECONFIG=<PATH>/tf/kubeconfig` to let kubectl know how to connect to cluster.

- policy that will allow worker nodes to join cluster `terraform output config-map-aws-auth`

### Destroy

As simple as `make destroy`.

## Configure kubectl

To allow kubectl to talk to your cluster `make kubeconfig` and follow the suggestion to export kube config path.

Make sure nodes can register `make config-map-aws-auth` and wait for nodes to appear.

Your are now ready.

## Kubernetes dashboard

Follow the documentation here https://docs.aws.amazon.com/eks/latest/userguide/dashboard-tutorial.html


### Working with multiple clusters at a time

By default, terraform creates one state file, that you will need to replace, if you are working with another cluster.
To smoothly work with multiple clusters, you can use terraform workspaces (https://www.terraform.io/docs/state/workspaces.html)

For example, you can create workspace by cluster name before provisioning cluster, and use `${terraform.workspace}`
as a cluster name  `cluster_name= "${terraform.workspace}-cluster"`

`terraform workspace new dummy`

view workspaces:

```
terraform workspace list
  default
* dummy
```

select specific workplace:

```
terraform workspace select dummy
```
