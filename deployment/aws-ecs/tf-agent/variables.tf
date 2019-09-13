variable "ecs_cluster_name" {
  description = "ECS Cluster Name"
  default = "ecs-default"
}

variable "sts_agent_task_family" {
  description = "Sts agent family"
  default = "stackstate-agent-task"
}

variable "sts_agent_image" {
  description = "version of agent to execute"
  default = "docker.io/stackstate/stackstate-agent-2-test:2.0.2.git.482.db7dccee"
}

variable "STS_URL" {
  description = "Stackstate server url"
}

variable "STS_API_KEY" {
  description = "Api key"
}

variable "STS_SKIP_SSL_VALIDATION" {
  description = "Skip ssl validation"
  default = "True"
}

variable "STS_PROCESS_AGENT_ENABLED" {
  description = "Enables process agent"
  default = "True"
}

 variable "STS_LOG_LEVEL" {
   description = "Log level"
   default = "debug"
 }
