resource "local_file" "rendered_task_def" {
  content     = "${data.template_file.sts_agent_taskdef.rendered}"
  filename = "${path.root}/rendered/sts_agent_taskdef.json"
}