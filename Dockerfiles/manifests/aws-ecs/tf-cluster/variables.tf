# options

variable "network_options_no_private" {
  default = true
}

variable "option_use_auto_scaling_group" {
  default = true
}

variable "option_use_manual_cluster_instances" {
  default = false
}

# check variables-autoscaling.tf
variable "scaling_alarm_actions_enabled" {
  default = false
}

# /options


# overrides

# provide alternative userdata for instances startup
variable "user_data" {
  default = false
}

variable "scaling_max_instance_size" {
  default = 4
}

variable "scaling_min_instance_size" {
  default = 1
}

variable "scaling_desired_capacity" {
  default = 2
}


# /overrides


variable "ecs_cluster" {
  description = "ECS cluster name"
  default = "ecs"
}

variable "ec2_key" {
  description = "key used on ec2 instances for troubleshouting"
  default = "voronenko_info"
}


locals {

  env = "${lookup(var.workspace_to_environment_map, terraform.workspace, "dev")}"
  region = "${var.environment_to_region_map[local.env]}"
  readable_env_name = "ecs-${local.env}"


  ecs_cluster_name = "${var.ecs_cluster}-${terraform.workspace}"

  app_instance_type = "${var.environment_to_instance_size_map[local.env]}"
}