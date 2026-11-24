build {
  name = "kb-include"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-kb-include"
    output_directory = "${var.output_root}/kb-include"
  }

  provisioner "microsoft-update" {
    include_kbs = [
      "KB890830",
      "KB2267602",
    ]
    report_path = "${local.report_directory}/kb-include.json"
  }
}

build {
  name = "kb-exclude"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-kb-exclude"
    output_directory = "${var.output_root}/kb-exclude"
  }

  provisioner "microsoft-update" {
    exclude_kbs = [
      "KB5034441",
    ]
    report_path = "${local.report_directory}/kb-exclude.json"
  }
}
