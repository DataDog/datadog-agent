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


Set the following environment variables:

```
export AWS_ACCESS_KEY_ID=
export AWS_SECRET_ACCESS_KEY=
export TF_VAR_AWS_SECRET_ACCESS_KEY=
export TF_VAR_AWS_ACCESS_KEY_ID=
export TF_VAR_AWS_REGION=us-east-1
export TF_VAR_CLUSTER_NAME=
export TF_VAR_SCALING_DESIRED_CAPACITY=
```

## Deploy

### Plan and apply

```bash
$ make plan
$ make apply
```

Plan will check what changes Terraform needs to apply, then apply deploys the changes.

To logon to the instances the private key can be generated from the terraform output.

```bash 
$ terraform output eks_rsa > eks_rsa 
```

The operation takes around 20 minutes.

### Configure

Once the deployment is done, the kubeconfig will let kubectl know how to connect to cluster:
```bash
$ make kubeconfig
run the suggested EXPORT
```

To make workers join the cluster they need to be have a role associated with it:
```bash
$ make config-map-aws-auth
```
and wait for nodes to appear.

Your are now ready.

### Destroy

As simple as `make destroy`.

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
