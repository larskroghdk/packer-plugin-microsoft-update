build {
  name = "all-updates"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-all-updates"
    output_directory = "${var.output_root}/all-updates"
  }

  provisioner "microsoft-update" {
    update_source = "microsoft-update"
    profile       = "all"

    include_feature_updates = false

    max_update_passes = 30
    max_reboots       = 25
    report_path       = "${local.report_directory}/all-updates.json"
  }
}
