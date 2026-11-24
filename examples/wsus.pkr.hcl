build {
  name = "wsus"

  sources = ["source.null.validation"]

  source "hyperv-vmcx.windows" {
    vm_name          = "mu-example-wsus"
    output_directory = "${var.output_root}/wsus"
  }

  provisioner "powershell" {
    environment_vars = ["WSUS_SERVER_URL=${var.wsus_server_url}"]
    inline = [
      "if (-not $env:WSUS_SERVER_URL) { throw 'Set the wsus_server_url variable.' }",
      "$policyPath = 'HKLM:\\SOFTWARE\\Policies\\Microsoft\\Windows\\WindowsUpdate'",
      "New-Item -Path \"$policyPath\\AU\" -Force | Out-Null",
      "Set-ItemProperty -Path $policyPath -Name WUServer -Value $env:WSUS_SERVER_URL",
      "Set-ItemProperty -Path $policyPath -Name WUStatusServer -Value $env:WSUS_SERVER_URL",
      "Set-ItemProperty -Path \"$policyPath\\AU\" -Name UseWUServer -Value 1 -Type DWord",
      "Restart-Service -Name wuauserv",
    ]
  }

  provisioner "microsoft-update" {
    update_source = "wsus"
    report_path   = "${local.report_directory}/wsus.json"
  }
}
