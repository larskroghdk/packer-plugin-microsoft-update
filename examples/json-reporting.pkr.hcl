build {
  name = "json-reporting"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-json-reporting"
    output_directory = "${var.output_root}/json-reporting"
  }

  provisioner "microsoft-update" {
    update_source = "default"
    fail_on_error = false
    log_level     = "debug"
    report_path   = "${local.report_directory}/json-reporting-${source.name}.json"
  }
}
