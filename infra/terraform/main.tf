variable "cloudflare_api_token" {}
variable "digitalocean_token" {}
variable "digitalocean_spaces_access_id" {}
variable "digitalocean_spaces_secret_key" {}

terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 3.0"
    }

    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }

  backend "s3" {
    bucket   = "bramble-tf"
    key      = "tf-state"
    endpoint = "nyc3.digitaloceanspaces.com"

    profile                     = "digitalocean-bramble"
    skip_region_validation      = true
    skip_credentials_validation = true
    region                      = "us-east-1"            // not used
  }
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "digitalocean" {
  token             = var.digitalocean_token
  spaces_access_id  = var.digitalocean_spaces_access_id
  spaces_secret_key = var.digitalocean_spaces_secret_key
}
