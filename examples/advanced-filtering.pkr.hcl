build {
  name = "advanced-filtering"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-advanced-filtering"
    output_directory = "${var.output_root}/advanced-filtering"
  }

  provisioner "microsoft-update" {
    update_source   = "microsoft-update"
    search_criteria = "IsInstalled=0 and IsHidden=0 and Type='Software'"

    classifications = [
      "security",
      "critical",
      "definition",
      "update-rollup",
      "e6cf1350-c01b-414d-a61f-263d14d133b4",
    ]

    exclude_titles = [
      "*Preview*",
      "*Silverlight*",
    ]

    max_updates_per_pass = 50
    max_install_attempts = 3
    max_retries          = 5
    retry_delay          = "30s"
    report_path          = "${local.report_directory}/advanced-filtering.json"
  }
}

build {
  name = "upstream-filters"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-upstream-filters"
    output_directory = "${var.output_root}/upstream-filters"
  }

  provisioner "microsoft-update" {
    filters = [
      "exclude:$_.Title -like '*Preview*'",
      "include:$true",
    ]
    update_limit = 25
    report_path  = "${local.report_directory}/upstream-filters.json"
  }
}
