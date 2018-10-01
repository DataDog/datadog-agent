# Kubernetes Cluster Setup

Important: setup follows https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html as of commit date.

In order to start with cluster setup, you will need:

A) AWS credentials with necessary rights
B) terraform tool itself
C) AWS authenticator for EKS, called heptio

```
curl -o <PATH>/heptio-authenticator-aws https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-06-05/bin/linux/amd64/heptio-authenticator-aws
curl -o <PATH>/heptio-authenticator-aws.md5 https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-06-05/bin/linux/amd64/heptio-authenticator-aws.md5
chmod +x <PATH>/heptio-authenticator-aws
```
D) AWS credentials should be placed to location, so it can be identified by AWS client tools and terraform.

One of the recommended options might be configuring those parameters through environment variables.

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

You can also put aws credentials for the AWS account into ~/.aws/credentials instead.
(Not recommended)

### Provision

If is the first time you provision with those script `terraform init`.

If you want to check what changes Terraform will apply you can see it with `terraform plan`.

Apply the changes `terraform apply`.

The provision takes around 20mins.

### Provisioning outcomes

Provisioning outcomes are:

1)  `terraform.tfstate` - required for proper dismounting of all the created resources on later stages
2)  `kubeconfig` file produced by `make kubelet-config`
You can `export KUBECONFIG=<PATH>/tf/kubeconfig` to let kubectl know how to connect to cluster.
3)  policy that will allow worker nodes to join cluster `terraform output config-map-aws-auth`


### Destroying

Assuming you have `terraform.tfstate` stored, as simple as `terraform destroy`

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


### Configure kubectl

To allow kubectl to talk to your cluster `make kubeconfig` and follow the suggestion to export kube config path.

Make sure nodes can register `make config-map-aws-auth` and wait for nodes to appear.

Your are now ready.

### Setup on an already provisioned Cluster

Get the kubectl command and aws-iam-authenticator command
Get the kubeconfig file from someone and put it somewhere (alternatively - get tfstate file, and proceed with `make kubeconfig`)

Get the aws credentials for the AWS account and put them in ~/.aws/credentials.

> export KUBECONFIG=<path_to_kubeconfig>


### Getting kubernetes dashboard

kube-dashboard normal install:

```sh
kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/master/src/deploy/recommended/kubernetes-dashboard.yaml
```

above installs dashboard with recommended security measures. For fully test cluster, you might want less secure install

```sh
kubectl create -f dashboard-admin.yaml
```

where dashboard-admin.yaml is

```yaml

apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: kubernetes-dashboard
  labels:
    k8s-app: kubernetes-dashboard
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: kubernetes-dashboard
  namespace: kube-system

```



### Access the dashboard

> kubectl proxy --port=8001
> browse to http://localhost:8001/api/v1/namespaces/kube-system/services/https:kubernetes-dashboard:/proxy/#!/overview?namespace=default
