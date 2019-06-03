variable "workspace_to_environment_map" {
  type = "map"
  default = {
    default = "default"
    dev     = "dev"
    qa      = "qa"
    staging = "staging"
    prod    = "prod"
  }
}

variable "environment_to_region_map" {
  type = "map"
  default = {
    default = "us-east-1"
    dev     = "us-east-1"
    qa      = "us-east-1"
    staging = "us-east-1"
    prod    = "us-east-1"
  }
}


variable "environment_to_instance_size_map" {
  type = "map"
  default = {
    default = "t2.small"
    dev     = "t2.small"
    qa      = "t2.small"
    staging = "t2.small"
    prod    = "t2.small"
  }
}