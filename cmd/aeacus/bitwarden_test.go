package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
)

type TestValues struct {
	mount, path string
	data        map[string]interface{}
}

var (
	bwValues = []TestValues{
		{
			path: "password/services/a",
			data: map[string]interface{}{
				"username": "hello",
				"password": "world",
			},
		},
		{
			path: "password/services/b",
			data: map[string]interface{}{
				"username": "world",
				"password": "hello",
			},
		},
		{
			path: "password/services/c",
			data: map[string]interface{}{
				"username": "foo",
				"password": "bar",
			},
		},
	}
	vaultValues = []TestValues{
		{
			mount: "secret",
			path:  "password/services/d",
			data: map[string]interface{}{
				"username": "hello",
				"password": "world",
			},
		},
		{
			mount: "secret",
			path:  "password/services/e",
			data: map[string]interface{}{
				"username": "world",
				"password": "hello",
			},
		},
		{
			mount: "secret",
			path:  "password/services/f",
			data: map[string]interface{}{
				"username": "foo",
				"password": "bar",
			},
		},
	}
)

func InitTestSetup(t *testing.T, vc *vault.Client, bc BitwardenClient) {
	t.Logf("populating vault with keys")
	for _, v := range vaultValues {
		err := VaultWrite(vc, v.mount, v.path, v.data)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("populating bitwarden with keys")
	for _, v := range bwValues {
		json_data, err := json.Marshal(map[string]interface{}{
			"folderId": nil,
			"type":     1,
			"name":     v.path,
			"login": struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}{
				Username: v.data["username"].(string),
				Password: v.data["password"].(string),
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal %s: %v", v.path, err)
		}
		bc.CreateItem(json_data)
	}
}

func CleanupTestSetup(t *testing.T, vc *vault.Client, bc BitwardenClient) {
	t.Logf("cleaning vault keys")
	for _, v := range vaultValues {
		err := VaultDestroy(vc, v.mount, v.path)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				t.Fatal(err)
			}
		}
	}
	for _, v := range bwValues {
		err := VaultDestroy(vc, v.mount, v.path)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				t.Fatal(err)
			}
		}
	}
	t.Logf("cleaning bitwarden keys")
	for _, v := range vaultValues {
		err := VaultDestroy(vc, v.mount, v.path)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				t.Fatal(err)
			}
		}
	}
	for _, v := range bwValues {
		err := VaultDestroy(vc, v.mount, v.path)
		if err != nil {
			if !strings.Contains(err.Error(), "404") {
				t.Fatal(err)
			}
		}
	}
}

func SetupClients(t *testing.T, ctx context.Context, internal Config) (vc *vault.Client, bc BitwardenClient) {
	// setup the accesss to vault from the client
	config := vault.DefaultConfig()
	config.Address = internal.Vault.Addr
	client, err := vault.NewClient(config)
	if err != nil {
		t.Fatal("failed to create default client")
	}

	// start up the token lifecycle management for vault
	errChan := make(chan error)
	if os.Getenv("VAULT_TOKEN") == "" {
		go renewToken(client, errChan)
	}

	if internal.Bitwarden.Local {
		go StartBWServe(context.Background())
		t.Logf("waiting for bw to start up")
		time.Sleep(5 * time.Second)
	}

	// start up interaction with Bitwarden
	bwClient, err := NewBitwardenClient(internal)
	if err != nil {
		t.Fatalf("failed to login to bw")
	}

	return client, bwClient
}

func TestBWSync(t *testing.T) {

	ctx := context.Background()

	internal, err := ParseConfig("../../config.json")
	if err != nil {
		t.Fatal("failed to parse config file")
	}

	vc, bc := SetupClients(t, ctx, internal)
	InitTestSetup(t, vc, bc)
	tests := []struct {
		mount, path string
	}{
		{"secret", "password/services/d"},
		{"secret", "password/services/e"},
		{"secret", "password/services/f"},
	}

	for _, tt := range tests {
		err := bc.BWSync(vc, tt.mount, tt.path)
		if err != nil {
			t.Fatalf("failed to run function: %v", err)
		}
	}

	CleanupTestSetup(t, vc, bc)
}
