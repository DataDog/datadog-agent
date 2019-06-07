
// ecs-service-role
resource "aws_iam_role" "ecs_service_role" {
  name                = "ecs_service_role_${local.env}"
  path                = "/"
  assume_role_policy  = "${data.aws_iam_policy_document.ecs-service-policy.json}"
}

resource "aws_iam_role_policy_attachment" "ecs-service-role-attachment" {
  role       = "${aws_iam_role.ecs_service_role.name}"
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceRole"
}

data "aws_iam_policy_document" "ecs-service-policy" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs.amazonaws.com"]
    }
  }
}

//ecs-instance-role

resource "aws_iam_role" "ecs_instance_role" {
  name                = "ecs_instance_role_${local.env}"
  path                = "/"
  assume_role_policy  = "${data.aws_iam_policy_document.ecs-instance-policy.json}"
}

data "aws_iam_policy_document" "ecs-instance-policy" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

//instance profile

resource "aws_iam_role_policy_attachment" "ecs-instance-role-attachment" {
  role       = "${aws_iam_role.ecs_instance_role.name}"
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_instance_profile" "ecs_instance_profile" {
  name = "ecs-instance-profile"
  path = "/"
  role = "${aws_iam_role.ecs_instance_role.id}"
  provisioner "local-exec" {
    command = "sleep 10"
  }
}

