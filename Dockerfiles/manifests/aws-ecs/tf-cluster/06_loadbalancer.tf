
# If you duplicate application load balancer creation in ansible
# Make sure for properites to match, to prevent infrastructure drift

resource "aws_lb" "ecs" {
  name               = "ecs-${local.env}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = ["${aws_security_group.vpc_security_groups_elb.id}"]
  subnets            = ["${aws_subnet.pub_subnet1.id}", "${aws_subnet.pub_subnet2.id}"]
  enable_deletion_protection = true

//  access_logs {
//    bucket  = "${aws_s3_bucket.lb_logs.bucket}"
//    prefix  = "ecs_${local.env}_alb"
//    enabled = true
//  }

  tags = {
    Environment = "${local.env}"
  }
}