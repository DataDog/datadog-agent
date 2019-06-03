resource "aws_ecs_cluster" "ecs_cluster" {
  name = "${local.ecs_cluster_name}"
}