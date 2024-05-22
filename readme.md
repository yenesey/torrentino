# Yet another telegram bot to manage my NAS download ecosystem

Integrates with:
- [Transmission](https://github.com/transmission/transmission) - for downloads
- [Jackett](https://github.com/Jackett/Jackett) - search engine for torrents
- [Torrserver](https://github.com/YouROK/TorrServer) - instant watch videos

### Installation
 - install [golang](https://go.dev/doc/install)
 - clone this repo and build binary:
\> git clone https://github.com/yenesey/torrentino.git
\> torrentino
\> go build


- create and fullfill settings.json by example:
```json
{
    "jackett" : {
        "host" : "host_name_or_ip",
        "port" : 9117,
        "api_key" : "***"
    },
    "transmission" : {
        "host" : "host_name_or_ip",
        "port" : 9091
    },
    "torrserver" : {
        "host" : "host_name_or_ip",
        "port" : 8090
    },
    "telegram_api_token" : "***",
    "users_list" : [],
    "download_dir" : ""
}
```
- don't forget to obtain and setup your own telegram_api_token (via @BotFather)

### Run
 - append your telegram user id to "users_list" and start bot
    - first run with empty "users_list" in config, you'll see ID on any interaction with bot

\> torrentino