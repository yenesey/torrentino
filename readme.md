# Go app for managing the “home” download ecosystem on a NAS.

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

- copy [`settings.example.json`](deploy/settings.example.json) to `settings.json` and replace
  every `REPLACE_WITH_...` placeholder; the example contains no real keys, tokens or
  passwords. The file also includes an optional `web` block (disabled by default).

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

### Web interface (optional)

The same binary can serve a browser UI in addition to Telegram. It is disabled by
default: without a `web` block the bot behaves exactly as before. Enable it in
`settings.json` by copying the `web` block from
[`settings.example.json`](deploy/settings.example.json), setting `"enabled": true`, and
generating a password hash below.

The web UI needs its own accounts; Telegram `users-list` is not reused, so login
works even when Telegram is unavailable. Generate a hash for every account with:

```bash
read -rs PASSWORD && printf %s "$PASSWORD" | ./torrentino -hash-password
```

The flag reads the password from stdin (not argv), does not load `settings.json`,
prints one line for `web.users[].password-hash` and exits. Passwords are stored only
as PBKDF2-HMAC-SHA256 hashes.

Fields:

- `web.listen` - listen address, default `127.0.0.1:8088`. In the recommended
  deployment it stays on loopback and nginx terminates TLS.
- `web.base-path` - external URL prefix. `"/"` for the subdomain deployment
  (`https://torrentino.example.com/`); `/torrentino` remains supported as a fallback
  for serving under a path.
- `web.page-size` - items per page, 5–100, default 20.
- `web.session.idle-timeout` / `max-age` - Go durations; sessions are in memory, so
  a restart logs everyone out.
- `web.secure-cookies` - set `true` when TLS is terminated by nginx.
- `web.trusted-proxies` - CIDRs whose `X-Forwarded-For` / `X-Forwarded-Proto`
  headers are trusted.
- `web.tls.cert` / `web.tls.key` - optional built-in TLS; set both or neither.
- `web.users` - one or more equal accounts. At least one is required when web is
  enabled.

Screens are `Find`, `Downloads` and `Torrserver` and mirror the bot: the same
sorting/filtering/pagination, the same actions, immediate deletion as in Telegram,
and a 5-second auto-refresh for downloads that pauses in a hidden tab. State is kept
server-side per session (one screen of each kind per session); there is no database.

The UI is reachable at `base-path` after login; `/healthz` is public and returns
`{"status":"ok"}`. When web is enabled and the Telegram bot cannot start, the process
logs the error and keeps serving the web UI.

This deployment is intended to run behind nginx at a private subdomain; the
docs use **https://torrentino.example.com/** as a placeholder. The app listens on
`127.0.0.1:8088` with `web.base-path: "/"`. The nginx site template is in
[`deploy/nginx/torrentino.conf`](deploy/nginx/torrentino.conf); the
old `https://example.com/torrentino/` path is redirected to the subdomain.
See [`deploy/nginx/`](deploy/nginx/) for the nginx deployment files.

### Run
 - append your telegram user id to "users-list" and start bot
    - first run with empty "users-list" in config, you'll see id in output on any interaction with bot
 - settings.json is read as `./settings.json`, so run the bot from the directory which contains it

```
run in command prompt
\> torrentino
```
