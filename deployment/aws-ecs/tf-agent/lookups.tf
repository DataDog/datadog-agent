data "aws_ecs_cluster" "monitored_cluster" {
  cluster_name = "${var.ecs_cluster_name}"
}