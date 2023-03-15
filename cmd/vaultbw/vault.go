package main

import (
	"context"
	"fmt"
	"log"
	"os"

	vault "github.com/hashicorp/vault/api"
	user "github.com/hashicorp/vault/api/auth/userpass"
)

// VaultSync synchronises the vault path with the provided ListResponse from bitwarden
// It fetches fata from Bitwarden and writes into Vault
func VaultSync(vc *vault.Client, resp ListResp, mount, prefix string) error {
	for _, v := range resp.Data.Data {
		name := v["name"].(string)
		data, err := DataFromBWItem(v)
		if err != nil {
			return fmt.Errorf("failed to parse BW item: %w", err)
		}

		err = VaultWrite(vc, mount, prefix+name, data)
		if err != nil {
			return fmt.Errorf("vault: failed to sync to vault: %w", err)
		}
	}
	return nil
}

// VaultWrite writes a single instance of data to the vault
func VaultWrite(vc *vault.Client, mount, path string, data map[string]interface{}) error {
	_, err := vc.KVv2(mount).Put(context.Background(), path, data)
	if err != nil {
		log.Fatalf("vault: unable to write secret: %v", err)
	}
	return nil
}

// renewToken is the asynchronoous function that will keep the access token alive for vault access
func renewToken(client *vault.Client) {
	for {
		vaultLoginResp, err := UserLogin(client)
		if err != nil {
			log.Fatalf("vault: unable to authenticate: %w", err)
		}
		tokenErr := manageTokenLifeCycle(client, vaultLoginResp)
		if tokenErr != nil {
			log.Fatalf("vault: failed to renew token: %w", err)
		}
	}
}

// manageTokenLifeCycle handles the lifecycle of the vault access token
func manageTokenLifeCycle(client *vault.Client, token *vault.Secret) error {
	renew := token.Auth.Renewable
	if !renew {
		log.Printf("vault: Token is not configured for renew, re-attempting login")
		return nil
	}

	watcher, err := client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    token,
		Increment: 3600,
	})
	if err != nil {
		return fmt.Errorf("vault: failed to initialize token lifetime watcher: %w", err)
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		case err := <-watcher.DoneCh():
			if err != nil {
				log.Printf("vault: failed to renew token, re-attempting login: %w", err)
				return nil
			}
			log.Printf("vault: failed to renew token, re-attempting login")
			return nil
		case renewal := <-watcher.RenewCh():
			log.Printf("vault: succesfully updated the token: %#v", renewal)
		}
	}
}

// UserLogin handles the login to vault
func UserLogin(client *vault.Client) (*vault.Secret, error) {
	userpassAuth, err := user.NewUserpassAuth(
		os.Getenv("VAULT_USER"),
		&user.Password{FromString: os.Getenv("VAULT_PASSWORD")},
	)

	if err != nil {
		return nil, fmt.Errorf("vault: unable to initialize userpass: %w", err)
	}

	authInfo, err := client.Auth().Login(context.Background(), userpassAuth)
	if err != nil {
		return nil, fmt.Errorf("vault: unable to login with userpass: %w", err)
	}
	if authInfo == nil {
		return nil, fmt.Errorf("vault: no authinfo returned after login: %w", err)
	}
	return authInfo, nil
}
