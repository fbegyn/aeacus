package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	"github.com/fbegyn/bitwarden-api"
	vault "github.com/hashicorp/vault/api"
	"golang.org/x/exp/slog"
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
	logger := slog.New(slog.NewTextHandler(os.Stdout).
		WithAttrs([]slog.Attr{slog.String("app", "aeacus"), slog.String("version", "development")}))

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	slog.SetDefault(logger)
	slog.Info("starting aeacus")

	internal, err := ParseConfig("./config.json")
	if err != nil {
		slog.Error("failed to parse config file")
	}

	// setup the accesss to vault from the client
	config := vault.DefaultConfig()
	config.Address = internal.Vault.Addr
	client, err := vault.NewClient(config)
	if err != nil {
		slog.Error("failed to create default client")
	}

	// start up the token lifecycle management for vault
	errChan := make(chan error)
	if os.Getenv("VAULT_TOKEN") == "" {
		go renewToken(client, errChan)
	}

	if internal.Bitwarden.Local {
		go bitwarden.StartBWServe(ctx)
		slog.Info("waiting for bw to start up", slog.String("component", "bitwarden"))
		time.Sleep(5 * time.Second)
	}

	bwConfig := bitwarden.Config{
		Addr: internal.Bitwarden.Addr,
	}
	// start up interaction with Bitwarden
	bwClient, err := bitwarden.NewBitwardenClient(bwConfig)
	if err != nil {
		slog.Error("failed to login to bw", err, slog.String("component", "bitwarden"))
		os.Exit(10)
	}

	// start actually doing sync stuff between Vault and Bitwarden
	// Sync Bitwarden passwords to vault
	for _, path := range internal.Vault.Paths {
		slog.Info("writing from bitwarden to vault", slog.String("vault-path", internal.Vault.MountPath), slog.String("bw-path", path), slog.String("bw-addr", internal.Bitwarden.Addr), slog.String("component", "vault"))
		// lookup the path in bitwarden
		resp, err := bwClient.List(path)
		if err != nil {
			slog.Error("failed to list bitwarden", err, slog.String("component", "vault"))
		}

		//pass it along to the vault client
		VaultSync(client, *resp, internal.Vault.MountPath, internal.Vault.Prefix)
	}

	// Sync Vault passwords to Bitwarden
	for _, path := range internal.Bitwarden.Paths {
		slog.Info("syncing from vault to bitwarden", slog.String("bw-path", path), slog.String("vault-addr", internal.Vault.Addr), slog.String("component", "bitwarden"))
		err := bwClient.BWSync(client, internal.Vault.MountPath, path)
		if err != nil {
			slog.Error("failure to sync to bitwarden", err, slog.String("component", "bitwarden"))
		}
	}

	slog.Info("locking vault", slog.String("component", "bitwarden"))
	err = bwClient.Lock()
	if err != nil {
		slog.Error("failed to lock bitwarden", err, slog.String("component", "bitwarden"))
	}
}
