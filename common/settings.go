package common

import (
	"encoding/json"
	"os"

	"torrentino/common/utils"

	"github.com/pkg/errors"
)

type hostPort struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SettingsStruct struct {
	Jackett struct {
		hostPort `json:",inline"`
		APIKey   string   `json:"api-key"`
		Indexers []string `json:"indexers"`
	} `json:"jackett"`

	Transmission hostPort `json:",inline"`
	Torrserver   hostPort `json:",inline"`

	TelegramAPIToken string  `json:"telegram-api-token"`
	UsersList        []int64 `json:"users-list"`

	Path struct {
		Default string `json:"default"`
		Movie   string `json:"movie"`
		Series  string `json:"series"`
	} `json:"path"`
}

var Settings SettingsStruct

func init() {
	data, err := os.ReadFile("./settings.json")
	if err != nil {
		utils.LogError(errors.Wrap(err, "readFile"))
	}
	err = json.Unmarshal(data, &Settings)
	if err != nil {
		utils.LogError(err)
	}
}
