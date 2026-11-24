build {
  name = "offline"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-offline"
    output_directory = "${var.output_root}/offline"
  }

  provisioner "microsoft-update" {
    update_source = "offline"

    dynamic "offline_package" {
      for_each = var.offline_package_paths
      content {
        source = offline_package.value
      }
    }

    report_path = "${local.report_directory}/offline.json"
  }
}
