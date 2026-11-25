build {
  name = "windows-server-modern"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-windows-server-modern"
    output_directory = "${var.output_root}/windows-server-modern"
  }

  provisioner "microsoft-update" {
    update_source = "microsoft-update"
    profile       = "recommended"
    exclude_titles = [
      "*Microsoft Edge*",
    ]
    report_path = "${local.report_directory}/windows-server-modern.json"
  }

  provisioner "windows-restart" {
    restart_timeout = "30m"
  }

  provisioner "microsoft-update" {
    update_source = "microsoft-update"
    report_path   = "${local.report_directory}/windows-server-modern-validation.json"
  }
}
