# if we go via autoscaling ....

resource "aws_launch_configuration" "ecs_launch_configuration" {
  name = "ecs_launch_configuration_${local.env}"
  image_id = "${data.aws_ami.ecs_ami.image_id}"
  instance_type = "${local.app_instance_type}"
  iam_instance_profile = "${aws_iam_instance_profile.ecs_instance_profile.id}"

  root_block_device {
    volume_type = "standard"
    volume_size = 30
    delete_on_termination = true
  }

//  lifecycle {
//    create_before_destroy = true
//  }

  security_groups = [ "${aws_security_group.vpc_security_groups_cluster.id}" ]
  associate_public_ip_address = "true"
  key_name = "${var.ec2_key}"
  user_data = "${data.template_file.user_data.rendered}"

}

resource "aws_autoscaling_group" "ecs_autoscaling_group" {
  name = "ecs_autoscaling_group_${local.env}"
  max_size = "${var.scaling_max_instance_size}"
  min_size = "${var.scaling_min_instance_size}"
  vpc_zone_identifier = ["${aws_subnet.pub_subnet1.id}", "${aws_subnet.pub_subnet2.id}"]
  desired_capacity = "${var.scaling_desired_capacity}"
  launch_configuration = "${aws_launch_configuration.ecs_launch_configuration.name}"
  health_check_type = "EC2"
  tags = [
            {
            "key" = "Name"
            "value" = "ecs_autoscaling_group_${local.env}"
            "propagate_at_launch" = true
            },
            {
            "key" = "env"
            "value" = "${local.env}"
            "propagate_at_launch" = true
            }
         ]
  depends_on = ["aws_launch_configuration.ecs_launch_configuration"]
}