build {
  name = "microsoft-update"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-microsoft-update"
    output_directory = "${var.output_root}/microsoft-update"
  }

  provisioner "microsoft-update" {
    update_source = "microsoft-update"

    include_optional_updates = false
    include_preview_updates  = false
    include_driver_updates   = false
    include_feature_updates  = false

    max_update_passes = 20
    max_reboots       = 15

    search_timeout  = "30m"
    install_timeout = "120m"
    reboot_timeout  = "60m"

    fail_on_error = true
    report_path   = "${local.report_directory}/microsoft-update.json"
  }
}
