# aeacus

This is a silly idea that I said in a meeting that I'm starting to regert but I
said it and now I have to deal with this cursed thing I'm creating.

Aeacus can sync between the different password/secret repos. It uses the same
semantics as the `cp` tool. See the usage and config file explanation below.

## Usage

Set the following env variables:

* `BW_PASSWORD`: password for the Bitwarden vault
* `VAULT_TOKEN`: Vault access token

and then just do:

```bash
aeacus <repo ID src> <repo ID dst>
```

## Config file

The config holds the sttings required for running the program. The
`bitwarden.local` setting will use a local install of the Bitwarden CLI to spin
an instance of `bw serve` to access the Bitwarden vault.

```json
{
  "bitwarden": {
    "local": true
  },
  "repos": [
    {
      "id": "vault-test",
      "type": "vault",
      "userfield": "username",
      "passfield": "password",
      "addr": "http://localhost:8200",
      "mountpath": "secret",
      "paths": [
        "password/services/a",
        "password/services/b",
        "password/services/c"
      ]
    },
    {
      "id": "bw-test",
      "type": "bitwarden",
      "userfield": "username",
      "passfield": "password",
      "addr": "http://localhost:8087",
      "paths": [
        "password/services/d",
        "password/services/e",
        "password/services/f"
      ]
    }
  ]
}
```
