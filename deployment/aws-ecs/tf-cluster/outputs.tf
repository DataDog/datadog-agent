locals {

  aws_commons_yml = <<YAML

# options
option_use_auto_scaling_group: ${var.option_use_auto_scaling_group}
option_use_yml_config: false  # set true for coreos - causes /etc/ecs/ecs.config creation
option_use_manual_cluster_instances: ${var.option_use_manual_cluster_instances}
network_options_no_private: true

# /options

readable_env_name: "${local.readable_env_name}"

ecsServiceRole_arn: "${aws_iam_role.ecs_service_role.arn}"
ecsInstanceRole_arn: "${aws_iam_role.ecs_instance_role.arn}"

aws_region: "${local.region}"

aws_vpc_id: ${aws_vpc.app_vpc.id}

aws_vpc_pubsubnet1: ${aws_subnet.pub_subnet1.id}
aws_vpc_pubsubnet2: ${aws_subnet.pub_subnet2.id}
aws_vpc_privsubnet1: ${aws_subnet.priv_subnet1.id}
aws_vpc_privsubnet2: ${aws_subnet.priv_subnet2.id}

aws_sg_elb: ${aws_security_group.vpc_security_groups_elb.id}
aws_sg_cluster: ${aws_security_group.vpc_security_groups_cluster.id}
aws_sg_datalayer: ${aws_security_group.vpc_security_groups_datalayer.id}
aws_sg_bastion: ${aws_security_group.vpc_security_groups_bastion.id}

aws_app_loadbalancer: ${aws_lb.ecs.name}

vpc_availability_zone_t1: "${data.aws_availability_zones.available.names[0]}"
vpc_availability_zone_t2: "${data.aws_availability_zones.available.names[1]}"

aws_primary_route_table: ${aws_default_route_table.default.id}
aws_igw: ${aws_internet_gateway.app_igw.id}

default_autoscaling_min_size: 1
default_autoscaling_desired_capacity: 1
default_autoscaling_max_size: 4


ec2_key: "${var.ec2_key}"

ecs_cluster_name: "${aws_ecs_cluster.ecs_cluster.name}" # ALLOWED TO BE SET EXTERNALLY
# ecs_engine_auth_data_token: "SPECIFY"  # todo: SET IT FROM SECURE VARS , cat ~/.docker/config.json
# ecs_engine_auth_data_email: "SPECIFY"  # todo:  SET IT FROM SECURE VARS




YAML

}


