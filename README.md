# Vault/Bitwarden sync WIP

This is a silly idea that I said in a meeting that I'm starting to regert but I
said it and now I have to deal with this cursed thing I'm creating.

Due to the current time, there is a small bug that causes the paths to be
reserved. That paths at `vault` will be synced FROM Bitwarden TO Vault. That
paths under `bitwarden` will be synced FROM Vault TO Bitwarden.

## Usage

Set the following env variables:

* `BW_PASSWORD`: password for the Bitwarden vault
* `VAULT_TOKEN`: Vault access token

and then just do:

```bash
go run ./cmd/vaultbw
```

## Config file

The config holds the sttings required for running the program.

```json
{
  "vault": {
    "userfield": "username",         # field in vault that holds the username
    "passfield": "password",         # field in vault that hilds the password
    "addr": "http://localhost:8200", # vault addr
    "mountpath": "secret",           # secrets k/v mount
    "paths": [                       # array of paths to sync from vault
      "password/services/a",
      "password/services/b",
      "password/services/c",
      "password/services/d"
    ]
  },
  "bitwarden": {
    "local": true,                   # let syncer manage `bw sync` command
    "addr": "http://localhost:8087", # BW api endpoint
    "paths": [                       # array of paths to sync from bitwarden
      "password/services/e",
      "password/services/f"
    ]
  }
}
```
