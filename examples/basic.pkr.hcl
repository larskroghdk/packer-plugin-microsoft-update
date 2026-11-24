build {
  name = "basic"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-basic"
    output_directory = "${var.output_root}/basic"
  }

  provisioner "microsoft-update" {
    update_source = "windows-update"
    report_path   = "${local.report_directory}/basic.json"
  }
}
