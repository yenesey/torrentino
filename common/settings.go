package common

import (
	"encoding/json"
	"log"
	"os"
)

type hostPort struct {
	Host string
	Port int
}

type SettingsStruct struct {
	Jackett struct {
		hostPort
		Api_key string
	}
	Transmission       hostPort
	Torrserver         hostPort
	Telegram_api_token string
	Users_list         []int
	Download_dir       string
}

var Settings SettingsStruct

func init() {
	var data, _ = os.ReadFile("./settings.json")
	err := json.Unmarshal(data, &Settings)
	if err != nil {
		log.Fatal(err)
	}
}
