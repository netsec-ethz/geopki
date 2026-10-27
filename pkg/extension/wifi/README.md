# The `wifi` extension

A GeoCertificate with this extension registers a WiFi network for the
certificate's areas. A client that sees the network (by its SSID) at a place
covered by the certificate can compare what the network presents with what its
operator registered: the security type it advertises, the keys of its captive
portal, the certificates of its enterprise authentication servers. A network
that copies a registered SSID but can't present the registered keys is an
imitation (an "evil twin").

The certificate's core binds the operator's `domain` to the areas. The network
itself is in `extensions.wifi`, and the certificate lists `wifi` as critical:
a client that doesn't implement this extension has to ignore the certificate.

## Example

A closed enterprise network whose authentication server is pinned to its CA
(abbreviated; the full certificate is in
[`testdata/enterprise-ca.json`](testdata/enterprise-ca.json)):

```json
{
  "certificate_id": "corp-staff-1",
  "domain": "corp.example",
  "areas": [{ "type": "MultiPolygon", "coordinates": [[[[10.0, 50.0], "..."]], []] }],
  "areas_altitude": [[0, 500]],
  "not_valid_after": "2030-01-01T00:00:00Z",
  "critical": ["wifi"],
  "extensions": {
    "wifi": {
      "version": 1,
      "operator": "Example Corp",
      "ssid": "Corp Staff",
      "auth_modes": ["wpa2-enterprise"],
      "enterprise": {
        "aaa": [{
          "realms": ["corp.example", "*.corp.example"],
          "eap_methods": [{ "method": "peap", "phase2": ["mschapv2"] }, { "method": "eap-tls" }],
          "servers": [{
            "names": ["radius.corp.example"],
            "ca_spki_sha256": ["<base64>"],
            "ca_certificates": ["<base64 DER>"]
          }]
        }]
      }
    }
  }
}
```

[`testdata/open-portal.json`](testdata/open-portal.json) is an open network with
a captive portal, [`testdata/federation-site.json`](testdata/federation-site.json)
a site of a roaming federation without local users.

## Fields

| Field | Meaning |
|---|---|
| `version` | Required, `1` |
| `operator` | Display name of the operator. Never used for matching |
| `ssid` | Required, at most 32 bytes. Clients compare it case-insensitively |
| `ssid_shared` | Default `false`: the SSID is exclusive in the areas, a network on it that doesn't match is an imitation. `true` for a generic SSID several operators use in one area |
| `auth_modes` | Required. Every mode the network advertises: `open`, `owe`, `wpa2-personal`, `wpa3-personal`, `wpa2-enterprise`, `wpa3-enterprise`, `wpa3-enterprise-192`. A transition-mode network lists all of them |
| `pmf` | `optional` or `required`; absent if unspecified |
| `enterprise` | Required with an enterprise mode, absent otherwise. See below |
| `portal` | The captive portal, if there is one. See below |

### `enterprise`

| Field | Meaning |
|---|---|
| `federation` | Name of a roaming federation the site belongs to; absent for a closed network, where every user is in `aaa` |
| `aaa` | Realm groups of the local users. May be empty only in a federation: a site without local users, where everyone is a visitor |
| `aaa[].realms` | NAI realms. `corp.example` matches exactly, `*.corp.example` subrealms only; case-insensitive. Required in a federation |
| `aaa[].eap_methods` | `{method, phase2[]}`, e.g. `peap` with `mschapv2`. For provisioning clients |
| `aaa[].servers[].names` | SAN dNSName (CN if none) of the RADIUS server's certificate |
| `aaa[].servers[].pinned_spki_sha256` | SHA-256 of the server certificate's SPKI, base64 |
| `aaa[].servers[].ca_spki_sha256` | SHA-256 of a CA's SPKI, base64 |
| `aaa[].servers[].ca_certificates` | The CA certificates themselves, base64 DER, for clients that have to pin the CA inside the handshake |

### `portal`

`domains[]` of `{name, role, pinned_spki_sha256[], certificate_sha256[]}`.
`role` is `primary` (the entry point), `delegate` (only reached after the
primary, e.g. a payment page) or `api` (the host of the Captive Portal API, RFC
8908). The pins are the trust anchor: a domain without pins can never be
verified.

## TODO (Check if necessary)

- **Exclusivity on insertion:** a certificate whose `(ssid, area)` overlaps
  one of another `domain`, without `ssid_shared`, should be refused (the
  same domain is allowed, to rotate keys). This needs a query against the
  stored certificates and is left for later.
- **Timestamps:** the core accepts any timestamp PostgreSQL does; WiFi
  certificates should use RFC 3339 in UTC (`...Z`).
- **BSSIDs** are deliberately not part of the format: they are easy to spoof
  and break deployments with many access points.
