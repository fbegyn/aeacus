package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"time"

	vault "github.com/hashicorp/vault/api"
)

type Config struct {
	Vault struct {
		Addr      string
		UserField string
		PassField string
		MountPath string
		Paths     []string
		Prefix    string
	}
	Bitwarden struct {
		Addr  string
		Paths []string
		Local bool
	}
}

func ParseConfig(path string) (Config, error) {
	filebuffer, err := ioutil.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}
	var conf Config
	json.Unmarshal(filebuffer, &conf)
	return conf, nil
}

func main() {
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()


	internal, err := ParseConfig("./config.json")
	if err != nil {
		log.Fatal("failed to parse config file")
	}

	// setup the accesss to vault from the client
	config := vault.DefaultConfig()
	config.Address = internal.Vault.Addr
	client, err := vault.NewClient(config)
	if err != nil {
		log.Fatal("failed to create fault client")
	}

	// start up the token lifecycle management for vault
	if os.Getenv("VAULT_TOKEN") == "" {
		go renewToken(client)
	}

	if internal.Bitwarden.Local {
		go StartBWServe(ctx)
		log.Print("waiting for bw serve to start up")
		time.Sleep(1 * time.Second)
	}

	// start up interaction with Bitwarden
	bwClient, err := NewBitwardenClient(internal)
	if err != nil {
		log.Fatalf("failed to login to bw: %v", err)
	}

	// start actually doing sync stuff between Vault and Bitwarden
	// Sync Bitwarden passwords to vault
	for _, path := range internal.Vault.Paths {
		log.Printf("vault: writing %s/%s from bitwarden %s\n", internal.Vault.MountPath, path, internal.Bitwarden.Addr)
		// lookup the path in bitwarden
		resp, err := bwClient.List(path)
		if err != nil {
			log.Fatalf("failed to list bitwarden: %v", err)
		}

		//pass it along to the vault client
		VaultSync(client, *resp, internal.Vault.MountPath, internal.Vault.Prefix)
	}

	// Sync Vault passwords to Bitwarden
	for _, path := range internal.Bitwarden.Paths {
		log.Printf("bitwarden: syncing %s from vault %s\n", path, internal.Vault.Addr)
		bwClient.BWSync(client, internal.Vault.MountPath, path)
	}

	log.Printf("bitwarden: locking vault")
	err = bwClient.Lock()
	if err != nil {
		log.Fatalf("failed to lock bitwarden: %v", err)
	}
}
