
resource "digitalocean_spaces_bucket" "bramble" {
  name   = "bramble"
  region = "nyc3"
  acl = "public-read"
}

resource "digitalocean_cdn" "bramble_store" {
  origin    = digitalocean_spaces_bucket.bramble.bucket_domain_name
}

# Output the endpoint for the CDN resource
output "fqdn" {
  value = digitalocean_cdn.bramble_store.endpoint
}

resource "digitalocean_spaces_bucket" "bramble-tf" {
  name   = "bramble-tf"
  region = "nyc3"
  acl = "private"
}
