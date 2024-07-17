package common

import (
	"encoding/json"
	"log"
	"os"

	"github.com/pkg/errors"
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

func logError(err error) {
	log.Printf("[common/settings] %s", err)
}

func init() {
	data, err := os.ReadFile("./settings.json")
	if err != nil {
		logError(errors.Wrap(err, "readFile"))
	}
	err = json.Unmarshal(data, &Settings)
	if err != nil {
		logError(errors.Wrap(err, "Unmarshal"))
	}
}
