package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/fbegyn/bitwarden-api"
	vault "github.com/hashicorp/vault/api"
	"golang.org/x/exp/slog"
)

type Config struct {
	Bitwarden struct {
		Local bool
	}
	Repos []Repo
}

type Repo struct {
	ID        string
	Type      string
	Addr      string
	UserField string
	PassField string
	Paths     []string
	Prefix    string
	MountPath string
}

func ParseConfig(path string) (Config, error) {
	filebuffer, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}
	var conf Config
	json.Unmarshal(filebuffer, &conf)
	return conf, nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	slog.SetDefault(logger)
	slog.Info("starting aeacus")

	internal, err := ParseConfig("./config.json")
	if err != nil {
		slog.Error("failed to parse config file")
	}

	// Parse command line arguments
	flag.Parse()
	args := flag.Args()
	srcRepoID := args[0]
	dstRepoID := args[1]

	// Determine the src/dst repo from the config file
	var srcRepo Repo
	var dstRepo Repo
	for _, v := range internal.Repos {
		if v.ID == srcRepoID {
			srcRepo = v
		}
		if v.ID == dstRepoID {
			dstRepo = v
		}
	}

	// Determine the src/dst repo types
	// TODO: Clearly this is a silly way and there should be a better option
	// than to create variable for each option. Maybe there is a use case for
	// generic or interfaces here. I should ask aroudn for better options,
	// but this will do for now since there are only 2 repo types.
	// Vault types
	srcVault := vault.DefaultConfig()
	dstVault := vault.DefaultConfig()
	if srcRepo.Type == "vault" {
		srcVault.Address = srcRepo.Addr
		srcClient, err := vault.NewClient(srcVault)
		if err != nil {
			slog.Error("failed to create default client")
		}

		// start up the token lifecycle management for vault
		errChan := make(chan error)
		if os.Getenv("VAULT_TOKEN") == "" {
			go renewToken(srcClient, errChan)
		}
	}
	if dstRepo.Type == "vault" {
		dstVault.Address = dstRepo.Addr
		dstClient, err := vault.NewClient(dstVault)
		if err != nil {
			slog.Error("failed to create default client")
		}

		// start up the token lifecycle management for vault
		errChan := make(chan error)
		if os.Getenv("VAULT_TOKEN") == "" {
			go renewToken(dstClient, errChan)
		}
	}

	// Bitwarden types
	if internal.Bitwarden.Local {
		go bitwarden.StartBWServe(ctx)
		slog.Info("waiting for bw to start up", slog.String("component", "bitwarden"))
		time.Sleep(5 * time.Second)
	}
	var srcBW bitwarden.BitwardenClient
	var dstBW bitwarden.BitwardenClient
	if srcRepo.Type == "bitwarden" {
		// start up interaction with Bitwarden
		srcBW, err = bitwarden.NewBitwardenClient(bitwarden.Config{
			Addr: srcRepo.Addr,
		})
		if err != nil {
			slog.Error("failed to login to bw", err, slog.String("component", "bitwarden"))
			os.Exit(10)
		}
	}
	if dstRepo.Type == "bitwarden" {
		// start up interaction with Bitwarden
		dstBW, err = bitwarden.NewBitwardenClient(bitwarden.Config{
			Addr: dstRepo.Addr,
		})
		if err != nil {
			slog.Error("failed to login to bw", err, slog.String("component", "bitwarden"))
			os.Exit(10)
		}
	}

	// Start the actual process of syncing things
	switch srcRepo.Type {
	case "vault":
		// Sync Vault passwords to Bitwarden
		switch dstRepo.Type {
		case "bitwarden":
			for _, path := range srcRepo.Paths {
				slog.Info("writing from vault to bitwarden",
					slog.String("vault-addr", srcRepo.Addr), slog.String("vault-mount", srcRepo.MountPath), slog.String("vault-path", path),
					slog.String("bw-addr", dstRepo.Addr), slog.String("bw-path", path),
					slog.String("source-id", srcRepo.ID), slog.String("source-type", srcRepo.Type),
					slog.String("destination-id", dstRepo.ID), slog.String("destination-type", dstRepo.Type),
				)
			}
		case "vault":
		default:
			slog.Error("unspecified dst repository type", slog.String("repo-type", srcRepo.Type))
		}
	case "bitwarden":
		client, err := vault.NewClient(dstVault)
		if err != nil {
			slog.Error("failed to create dst vault client", err, slog.String("component", "bitwarden"))
		}
		for _, path := range srcRepo.Paths {
			// lookup the path in bitwarden
			resp, err := srcBW.ListItems(path)
			if err != nil {
				slog.Error("failed to list bitwarden", err, slog.String("component", "bitwarden"))
			}
			if len(resp) == 0 {
				slog.Error("no bitwarden item was returned", slog.String("component", "bitwarden"), slog.String("path", path))
				continue
			}
			if len(resp) > 1 {
				slog.Error(
					"more than 1 bitwarden item was returned, please clean up any duplicates in the bw vault",
					slog.String("component", "bitwarden"), slog.String("path", path),
				)
				continue
			}
			// Sync Bitwarden passwords to vault
			switch dstRepo.Type {
			case "vault":
				slog.Info("writing from bitwarden to vault",
					slog.String("bw-addr", srcRepo.Addr), slog.String("bw-path", path),
					slog.String("vault-addr", dstRepo.Addr), slog.String("vault-mount", dstRepo.MountPath), slog.String("vault-path", path),
					slog.String("source-id", srcRepo.ID), slog.String("source-type", srcRepo.Type),
					slog.String("destination-id", dstRepo.ID), slog.String("destination-type", dstRepo.Type),
				)
				data, err := bitwarden.DataFromBWItem(resp[0])
				if err != nil {
					slog.Error("failed to convert bitwarden item into map", err, slog.String("component", "bitwarden"))
				}

				_, err = client.KVv2(dstRepo.MountPath).Put(ctx, path, data)
				if err != nil {
					slog.Error("failed to PUT data into vault", err, slog.String("component", "bitwarden"))
				}
			default:
				slog.Error("unspecified dst repository type", slog.String("repo-type", srcRepo.Type))
			}
		}
	default:
		slog.Error("unspecified src repository type", slog.String("repo-type", srcRepo.Type))
	}

	if srcRepo.Type == "bitwarden" {
		err = srcBW.Lock()
		if err != nil {
			slog.Error("failed to lock src bitwarden", err, slog.String("component", "bitwarden"))
		}
	}
	if dstRepo.Type == "bitwarden" {
		err = dstBW.Lock()
		if err != nil {
			slog.Error("failed to lock dst bitwarden", err, slog.String("component", "bitwarden"))
		}
	}
}
