//  Wherever possible, we will use a common set of tags for resources. This
//  makes it much easier to set up resource based billing, tag based access,
//  resource groups and more.
//
//  We are also required to set certain tags on resources to support Kubernetes
//  and AWS integration, which is needed for dynamic volume provisioning.
//
//  This is quite fiddly, the following resources should be useful:
//
//    - Terraform: Local Values: https://www.terraform.io/docs/configuration/locals.html
//    - Terraform: Default Tags for Resources in Terraform: https://github.com/hashicorp/terraform/issues/2283
//    - Terraform: Variable Interpolation for Tags: https://github.com/hashicorp/terraform/issues/14516
//    - OpenShift: Cluster Labelling Requirements: https://docs.openshift.org/latest/install_config/configuring_aws.html#aws-cluster-labeling

//  Define our common tags.
//   - Project: Purely for my own organisation, delete or change as you like!
//   - KubernetesCluster: Set to <cluster_id>, required for OpenShift < 3.7
//   - kubernetes.io/cluster/<name>: Set to <cluster_id>, required for OpenShift >= 3.7
//  The syntax below is ugly, but needed as we are using dynamic key names.
locals {
  common_tags = "${map(
    "Project", "openshift",
    "KubernetesCluster", "${var.cluster_name}",
    "kubernetes.io/cluster/${var.cluster_name}", "${var.cluster_id}"
  )}"
}
