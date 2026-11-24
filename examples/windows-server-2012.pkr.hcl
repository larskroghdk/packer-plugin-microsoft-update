build {
  name = "windows-server-2012"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-windows-server-2012"
    output_directory = "${var.output_root}/windows-server-2012"
  }

  provisioner "microsoft-update" {
    update_source = "microsoft-update"

    dynamic "offline_package" {
      for_each = zipmap(var.legacy_prerequisite_paths, var.legacy_prerequisite_checksums)
      content {
        guest_path = offline_package.key
        sha256     = offline_package.value
      }
    }

    max_update_passes = 25
    max_reboots       = 20
    search_timeout    = "60m"
    install_timeout   = "180m"
    report_path       = "${local.report_directory}/windows-server-2012.json"
  }
}
