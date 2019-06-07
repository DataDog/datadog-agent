
resource "aws_iam_role" "ecs-sts-agent-role" {
  name = "ecs-sts-agent-role"
  assume_role_policy = "${data.aws_iam_policy_document.ecs-task-role-assume.json}"
}

resource "aws_iam_role_policy_attachment" "ecs-sts-agent-policy-attach" {
  role = "${aws_iam_role.ecs-sts-agent-role.name}"
  policy_arn = "${aws_iam_policy.ecs-sts-agent-role-policy.arn}"
}

resource "aws_iam_policy" "ecs-sts-agent-role-policy" {
  name = "ecs-sts-agent-role-policy"
  policy = "${data.aws_iam_policy_document.ecs-sts-agent-role.json}"
}

data "aws_iam_policy_document" "ecs-sts-agent-role" {

  statement {
    sid = "AllowStsAgentToReadECSMetrics",
    effect = "Allow"
    actions = [
      "ecs:ListClusters",
      "ecs:ListContainerInstances",
      "ecs:DescribeContainerInstances"
    ]
    resources = [ "*" ]
  }

}

data "aws_iam_policy_document" "ecs-task-role-assume" {
  statement {
    effect = "Allow"
    actions = [ "sts:AssumeRole" ]
    principals {
      type = "Service"
      identifiers = [ "ecs-tasks.amazonaws.com" ]
    }
  }
}