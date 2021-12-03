

data "cloudflare_zone" "bramble_run" {
  name = "bramble.run"
}

resource "cloudflare_record" "bramble_run" {
  zone_id = data.cloudflare_zone.bramble_run.id
  name    = "bramble.run"
  value   = "maxmcd.github.io"
  type    = "CNAME"
}

resource "cloudflare_record" "store_bramble_run" {
  zone_id = data.cloudflare_zone.bramble_run.id
  name    = "store.bramble.run"
  value   = "maxmcd.github.io"
  type    = "CNAME"
  proxied = true
}

resource "cloudflare_page_rule" "store_redirect" {
  zone_id = data.cloudflare_zone.bramble_run.id
  target = "store.bramble.run/*"

  actions {
    forwarding_url {
      url = "https://bramble.nyc3.cdn.digitaloceanspaces.com/$1"
      status_code = 302
    }
  }
}


