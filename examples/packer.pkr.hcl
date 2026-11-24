packer {
  required_version = ">= 1.11.0"

  required_plugins {
    hyperv = {
      source  = "github.com/hashicorp/hyperv"
      version = ">= 1.1.4"
    }
    microsoft-update = {
      source  = "github.com/larskroghdk/microsoft-update"
      version = ">= 0.1.0"
    }
  }
}

variable "clone_from_vm_name" {
  type        = string
  description = "Name of a prepared Hyper-V VM (WinRM enabled) that the examples clone."
}

variable "switch_name" {
  type        = string
  default     = "Default Switch"
  description = "Hyper-V virtual switch with internet or WSUS access."
}

variable "winrm_username" {
  type    = string
  default = "Administrator"
}

variable "winrm_password" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Set with the PKR_VAR_winrm_password environment variable for builds; never commit it. Validation needs no password."
}

variable "winrm_use_ssl" {
  type    = bool
  default = false
}

variable "cpus" {
  type    = number
  default = 2
}

variable "memory" {
  type    = number
  default = 4096
}

variable "skip_export" {
  type    = bool
  default = true
}

variable "output_root" {
  type    = string
  default = "output"
}

variable "wsus_server_url" {
  type        = string
  default     = ""
  description = "WSUS server URL for the wsus example, for example https://wsus.example.com:8531."
}

variable "offline_package_paths" {
  type        = list(string)
  default     = []
  description = "Host paths of .msu or .cab packages for the offline example."
}

variable "legacy_prerequisite_paths" {
  type        = list(string)
  default     = []
  description = "Guest paths of servicing stack and SHA-2 prerequisite packages for legacy systems."
}

variable "legacy_prerequisite_checksums" {
  type        = list(string)
  default     = []
  description = "SHA-256 checksums of legacy_prerequisite_paths, in the same order."
}

source "null" "validation" {
  communicator = "none"
}

source "hyperv-vmcx" "windows" {
  clone_from_vm_name = var.clone_from_vm_name
  switch_name        = var.switch_name
  cpus               = var.cpus
  memory             = var.memory
  headless           = true
  skip_export        = var.skip_export
  communicator       = "winrm"
  winrm_username     = var.winrm_username
  winrm_password     = var.winrm_password
  winrm_use_ssl      = var.winrm_use_ssl
  winrm_insecure     = var.winrm_use_ssl
  winrm_timeout      = "2h"
  shutdown_command   = "shutdown /s /t 0 /f /d p:4:1 /c \"Packer shutdown\""
  shutdown_timeout   = "30m"
}

locals {
  report_directory = "${path.root}/artifacts"
}
