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

Set the following environment variables (use the molecule user credentials):

```
export AWS_ACCESS_KEY_ID=
export AWS_SECRET_ACCESS_KEY=
export TF_VAR_CLUSTER_NAME=
```

## Deploy

### Plan and apply

```bash
$ make plan
$ make apply
```

Plan will check what changes Terraform needs to apply, then apply deploys the changes.

The operation takes around 20 minutes.

### Configure

Once the deployment is done, the kubeconfig will let kubectl know how to connect to cluster:
```bash
$ make kubeconfig
```
Run the suggested EXPORT from the previous command.

To make workers join the cluster they need to be have a role associated with it:
```bash
$ make config-map-aws-auth
```
and wait for nodes to appear.

Your are now ready.

### Login into worker instances

To login into the instances the private key can be generated from the terraform output:

```bash
$ make private-key
```

Prior login you would need to switch the gateway used by the workers private route table from the nat to the internet one
 and open the SSH port on the worker security group.

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
