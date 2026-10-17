# Yet another telegram bot to manage my NAS download ecosystem

Integrates with:
- [Transmission](https://github.com/transmission/transmission) - for downloads
- [Jackett](https://github.com/Jackett/Jackett) - search engine for torrents
- [Torrserver](https://github.com/YouROK/TorrServer) - instant watch videos

### Installation
 - install [golang](https://go.dev/doc/install)
 - clone this repo and build binary:
```
\> git clone https://github.com/yenesey/torrentino.git
\> cd torrentino
\> go build
```

- create and fill settings.json by example:
```json
{
    "jackett" : {
        "host" : "host_name_or_ip",
        "port" : 9117,
        "api-key" : "***",
        "indexers" : ["rutor", "rutracker", "kinozal"],
        "timeout" : "60s"
    },
    "transmission" : {
        "host" : "host_name_or_ip",
        "port" : 9091
    },
    "torrserver" : {
        "host" : "host_name_or_ip",
        "port" : 8090
    },
    "telegram-api-token" : "***",
    "users-list" : [],
    "path" : {
        "default" : "/mnt/big/download/default",
        "movie" : "/mnt/big/download/movie",
        "series" : "/mnt/big/download/series"
    }
}
```

Where:
 - `telegram-api-token` - bot token, obtain your own via [@BotFather](https://t.me/BotFather)
 - `users-list` - telegram user ids allowed to use the bot
 - `jackett.api-key` - API key from the Jackett dashboard
 - `jackett.indexers` - Jackett tracker ids to search in; an empty list sends no
   tracker filter, so all configured indexers are queried
 - `jackett.timeout` - how long to wait for a Jackett response, as a Go duration
   (`"60s"`, `"1m30s"`, `"500ms"`); defaults to 60s, because Jackett queries the
   indexers one by one and a broad search over slow trackers takes a while.
   A value without a unit (`"60"`) is rejected at startup
 - `path.default` / `path.movie` / `path.series` - download directories used by the
   three download buttons in search results

Required fields are `telegram-api-token`, `jackett.api-key` and `host`/`port` of all
three services: if any of them is missing or invalid, the bot prints the list of
problems and exits instead of starting with broken endpoints.

### Run
 - append your telegram user id to "users-list" and start bot
    - first run with empty "users-list" in config, you'll see id in output on any interaction with bot
 - settings.json is read as `./settings.json`, so run the bot from the directory which contains it

```
run in command prompt
\> torrentino
```
