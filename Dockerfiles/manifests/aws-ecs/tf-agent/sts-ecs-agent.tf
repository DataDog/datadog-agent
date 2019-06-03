data "template_file" "sts_agent_taskdef_containers" {
  template = "${file("templates/sts_agent_containers_subtemplate.json.tpl")}"

  vars {
    sts_agent_image      = "${var.sts_agent_image}"
    STS_API_KEY          = "${var.STS_API_KEY}"
    STS_URL              = "${var.STS_URL}"
    STS_PROCESS_AGENT_ENABLED = "${var.STS_PROCESS_AGENT_ENABLED}"
    STS_SKIP_SSL_VALIDATION = "${var.STS_SKIP_SSL_VALIDATION}"
    LOG_LEVEL             =  "${var.STS_LOG_LEVEL}"
    sts_agent_task_family= "${var.sts_agent_task_family}"
  }
}

data "template_file" "sts_agent_taskdef" {
  template = "${file("templates/sts_agent_taskdef.json.tpl")}"

  vars {
    sts_agent_image      = "${var.sts_agent_image}"
    STS_API_KEY          = "${var.STS_API_KEY}"
    STS_URL              = "${var.STS_URL}"
    STS_SKIP_SSL_VALIDATION = "${var.STS_SKIP_SSL_VALIDATION}"
    STS_PROCESS_AGENT_ENABLED = "${var.STS_PROCESS_AGENT_ENABLED}"
    LOG_LEVEL            =  "${var.STS_LOG_LEVEL}"
    sts_agent_task_family= "${var.sts_agent_task_family}"
    sts_agent_taskdef_containers = "${data.template_file.sts_agent_taskdef_containers.rendered}"
  }
}

resource "aws_ecs_task_definition" "sts_agent" {
  family = "${var.sts_agent_task_family}"
  volume {
    name = "docker_sock"
    host_path = "/var/run/docker.sock"
  }
  volume {
    name = "proc"
    host_path = "/proc/"
  }
  volume {
    name = "cgroup"
    host_path = "/cgroup/"
  }
  volume {
    name = "passwd"
    host_path = "/etc/passwd"
  }
  volume {
    name = "kerneldebug"
    host_path = "/sys/kernel/debug"
  }

  container_definitions = "${data.template_file.sts_agent_taskdef_containers.rendered}"
}


resource "aws_ecs_service" "sts_agent" {
  name                = "sts_agent"
  cluster             = "${data.aws_ecs_cluster.monitored_cluster.id}"
  task_definition     = "${aws_ecs_task_definition.sts_agent.arn}"
//  iam_role            = "${aws_iam_role.ecs-sts-agent-role.arn}"
  scheduling_strategy = "DAEMON"
}
