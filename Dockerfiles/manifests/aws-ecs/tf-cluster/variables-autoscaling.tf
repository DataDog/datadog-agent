variable "scaling_adjustment_up" {
  default     = "1"
  description = "How many instances to scale up by when triggered"
}

variable "scaling_adjustment_down" {
  default     = "-1"
  description = "How many instances to scale down by when triggered"
}

variable "scaling_metric_name" {
  default     = "CPUReservation"
  description = "Options: CPUReservation or MemoryReservation"
}

variable "scaling_adjustment_type" {
  default     = "ChangeInCapacity"
  description = "Options: ChangeInCapacity, ExactCapacity, and PercentChangeInCapacity"
}

variable "scaling_policy_cooldown" {
  default     = 300
  description = "The amount of time, in seconds, after a scaling activity completes and before the next scaling activity can start."
}

variable "scaling_evaluation_periods" {
  default     = "2"
  description = "The number of periods over which data is compared to the specified threshold."
}

variable "scaling_alarm_period" {
  default     = "120"
  description = "The period in seconds over which the specified statistic is applied."
}

variable "scaling_alarm_threshold_up" {
  default     = "100"
  description = "The value against which the specified statistic is compared."
}

variable "scaling_alarm_threshold_down" {
  default     = "50"
  description = "The value against which the specified statistic is compared."
}

