package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cli/oauth/api"
	"github.com/kirsle/configdir"
)

// getDiff will return the diff of the current
// workspace git repository.
func getDiff() ([]byte, error) {
	cmd := exec.Command("git", "diff")
	buf := &bytes.Buffer{}
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// storeToken will store the access token in the
// diffshare config folder.
func storeToken(token *api.AccessToken) error {
	path := tokenPath()
	_, err := os.Stat(path)
	if err == nil {
		// delete the existing file.
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	buf, _ := json.Marshal(token)
	return ioutil.WriteFile(path, buf, 0755)
}

// getToken will return the token
func getToken() (*api.AccessToken, error) {
	path := tokenPath()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	token := &api.AccessToken{}
	if err := json.Unmarshal(buf, token); err != nil {
		return nil, err
	}
	return token, nil
}

func tokenPath() string {
	cfgPath := configdir.LocalConfig("diffshare")
	return filepath.Join(cfgPath, "token.json")
}

func createConfigDir() error {
	cfgPath := configdir.LocalConfig("diffshare")
	_, err := os.Stat(cfgPath)
	if !os.IsNotExist(err) {
		return err
	}
	return os.Mkdir(cfgPath, 0755)
}

func StringPtr(in string) *string {
	return &in
}
