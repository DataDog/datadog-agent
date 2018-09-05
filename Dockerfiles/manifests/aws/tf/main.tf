//EKS Cluster: AWS managed Kubernetes cluster
//AutoScaling Group containing 2 t2.medium instances based on the latest EKS Amazon Linux 2
//AMI: Operator managed Kuberneted worker nodes for running Kubernetes service deployments
//Associated VPC, Internet Gateway, Security Groups, and Subnets:
//Operator managed networking resources for the EKS Cluster and worker node instances
//Associated IAM Roles and Policies:
//Operator managed access resources for EKS and worker node instances