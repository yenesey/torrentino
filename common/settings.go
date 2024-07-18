package common

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	"torrentino/common/utils"
)

type hostPort struct {
	Host string
	Port int
}

type SettingsStruct struct {
	Jackett struct {
		hostPort
		Api_key  string
		Indexers []string
	}
	Transmission       hostPort
	Torrserver         hostPort
	Telegram_api_token string
	Users_list         []int64
	Download_dir       string
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
