package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	vault "github.com/hashicorp/vault/api"
	"golang.org/x/exp/slog"
)

// StartBWServe starts up the local API endpoint in the background and
// supervises it. It also maintains the lifecycle.
func StartBWServe(ctx context.Context) {
	cmd := exec.Command("bw", "serve")
	if err := cmd.Start(); err != nil {
		slog.Error("failed to run 'bw serve'", err, slog.String("component", "bitwarden"))
	}
	<-ctx.Done()
	cmd.Process.Kill()
}

func (bw *BitwardenClient) BWSync(client *vault.Client, mount, path string) error {
	secret, err := client.KVv2(mount).Get(context.Background(), path)
	if err != nil {
		return fmt.Errorf("unable to read secret from vault: %v", err)
	}
	userValue, ok := secret.Data[bw.UserField].(string)
	if !ok {
		return fmt.Errorf("value type assertion failed for user field")
	}

	passValue, ok := secret.Data[bw.PassField].(string)
	if !ok {
		return fmt.Errorf("value type assertion failed for password field")
	}

	fmt.Println(userValue)
	fmt.Println(passValue)

	json_data, err := json.Marshal(map[string]interface{}{
		"folderId": nil,
		"type":     1,
		"name":     path,
		"login": struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}{
			Username: userValue,
			Password: passValue,
		},
	})
	bw.CreateItem(json_data)
	return nil
}

type CreateResp struct {
	Success      bool
	Data         map[string]interface{}
	RevisionDate time.Time
	DeleteDate   time.Time
}

func (bw *BitwardenClient) CreateItem(item []byte) error {
	req, err := http.NewRequest("POST", bw.BaseURL+"/object/item", bytes.NewBuffer(item))
	if err != nil {
		return fmt.Errorf("bitwarden: failed to create request for BW: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := bw.Client.Do(req)
	if err != nil {
		return fmt.Errorf("bitwarden: failed to write password to bitwarden: %w", err)
	}
	defer resp.Body.Close()
	var createResp CreateResp
	json.NewDecoder(resp.Body).Decode(&createResp)
	if !createResp.Success {
		return fmt.Errorf("bitwarden: failed to create object: %v", resp)
	}
	return nil
}

type DeleteResp struct {
	Success bool
}

func (bw *BitwardenClient) DeleteItem(item []byte) error {
	req, err := http.NewRequest("DELETE", bw.BaseURL+"/object/item", bytes.NewBuffer(item))
	if err != nil {
		return fmt.Errorf("bitwarden: failed to delete request for BW: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := bw.Client.Do(req)
	if err != nil {
		return fmt.Errorf("bitwarden: failed to delete password from bitwarden: %w", err)
	}
	defer resp.Body.Close()
	var createResp DeleteResp
	json.NewDecoder(resp.Body).Decode(&createResp)
	if !createResp.Success {
		return fmt.Errorf("bitwarden: failed to delete object: %v", resp)
	}
	return nil
}

type BitwardenClient struct {
	BaseURL      string
	SessionToken string
	Client       *http.Client
	UserField    string
	PassField    string
}

type UnlockResp struct {
	Success bool
	Data    map[string]interface{}
}

func NewBitwardenClient(conf Config) (BitwardenClient, error) {
	bwClient := BitwardenClient{
		BaseURL:   conf.Bitwarden.Addr,
		UserField: conf.Vault.UserField,
		PassField: conf.Vault.PassField,
	}

	if os.Getenv("BW_PASSWORD") == "" {
		return BitwardenClient{}, fmt.Errorf("bitwarden password not set in BW_PASSWORD")
	}
	loginCred := map[string]string{
		"password": os.Getenv("BW_PASSWORD"),
	}

	json_data, err := json.Marshal(loginCred)
	if err != nil {
		return BitwardenClient{}, fmt.Errorf("failed to encode login creds: %w", err)
	}

	resp, err := http.Post(bwClient.BaseURL+"/unlock", "application/json", bytes.NewBuffer(json_data))
	if err != nil {
		return BitwardenClient{}, fmt.Errorf("unable to authenticate to BW host: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp UnlockResp
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	bwClient.SessionToken = tokenResp.Data["raw"].(string)
	bwClient.Client = &http.Client{}
	return bwClient, nil
}

func (bw *BitwardenClient) Lock() error {
	resp, err := http.Post(bw.BaseURL+"/lock", "application/json", &bytes.Buffer{})
	if err != nil {
		return fmt.Errorf("unable to lock vault: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

type ListResp struct {
	Success bool
	Data    struct {
		Object string
		Data   []map[string]interface{}
	}
}

// DataFromBWItem has the goal to convert data from a Bitwarden item to be ready
// to be inserted into Vault
func DataFromBWItem(resp map[string]interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	if resp["login"] != nil {
		login := resp["login"].(map[string]interface{})
		if login["username"] != nil {
			data["username"] = login["username"]
		} else {
			return nil, fmt.Errorf("bw item has no username")
		}
		if login["password"] != nil {
			data["password"] = login["password"]
		} else {
			return nil, fmt.Errorf("bw item has no password")
		}
	}
	return data, nil
}

// List implements the objects items list API functionality. It retturns a
// ListResponse according to the Bitwarden API.
func (bw *BitwardenClient) List(search string) (*ListResp, error) {
	req, err := http.NewRequest("GET", bw.BaseURL+"/list/object/items", nil)
	q := req.URL.Query()
	q.Add("search", search)
	req.URL.RawQuery = q.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bitwarden: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := bw.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to list items in vault: %w", err)
	}
	defer resp.Body.Close()
	var listResp ListResp
	json.NewDecoder(resp.Body).Decode(&listResp)
	return &listResp, nil
}
