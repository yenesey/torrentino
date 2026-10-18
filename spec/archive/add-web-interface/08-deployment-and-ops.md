# 08. Конфигурация, деплой и эксплуатация

## 1. Конфигурация

В `settings.json` добавляется объект `web`. При `enabled: false` (по умолчанию) поведение
бота не меняется; валидация веб-полей не выполняется.

Основной деплой — отдельный поддомен `torrentino.example.com`:
`base-path: "/"`, TLS на nginx, приложение слушает только loopback.

```json
{
  "web": {
    "enabled": true,
    "listen": "127.0.0.1:8088",
    "base-path": "/",
    "page-size": 20,
    "session": {
      "idle-timeout": "12h",
      "max-age": "168h"
    },
    "secure-cookies": true,
    "tls": {
      "cert": "",
      "key": ""
    },
    "trusted-proxies": ["127.0.0.1/32"],
    "users": [
      {
        "name": "denis",
        "password-hash": "pbkdf2-sha256$600000$<base64-salt>$<base64-hash>"
      },
      {
        "name": "family",
        "password-hash": "pbkdf2-sha256$600000$<base64-salt>$<base64-hash>"
      }
    ]
  }
}
```

| Поле | По умолчанию | Смысл |
|---|---|---|
| `enabled` | `false` | включает HTTP-сервер |
| `listen` | `127.0.0.1:8088` | адрес/порт; для деплоя — loopback |
| `base-path` | `/` | внешний префикс URL; основной деплой — `/`, fallback — `/torrentino` |
| `page-size` | `20` | элементов на страницу, 5–100 |
| `session.idle-timeout` | `12h` | простой до разлогина, Go-duration |
| `session.max-age` | `168h` | абсолютный срок жизни сессии |
| `secure-cookies` | `false` | ставить `Secure`; за TLS-прокси — `true` |
| `tls.cert` / `tls.key` | пусто | PEM-файлы для встроенного TLS |
| `trusted-proxies` | `[]` | CIDR прокси, которым верим в `X-Forwarded-*` |
| `users` | `[]` | аккаунты; при `enabled: true` нужен хотя бы один |

`users` может содержать несколько равноправных аккаунтов; у каждого свой пароль и свои
сессии.

Валидация при загрузке (fail-fast, общий список проблем в стиле `common/settings.go`):

- `listen` разбирается как `host:port`, порт 1–65535;
- `base-path` начинается с `/`, без хвостового `/` (кроме корня), без `//`, `..`,
  пробелов, query/fragment; нормализуется, иначе — ошибка;
- при `enabled: true` — хотя бы один пользователь, уникальные имена (без учёта регистра),
  корректный формат `password-hash` (PBKDF2, iterations 100 000–2 000 000);
- `tls.cert` и `tls.key` — либо оба, либо ни одного (файлы проверяются при старте TLS);
- `secure-cookies: true` не требует встроенного TLS: за прокси это обязательный режим,
  флаг ставится независимо от `tls.*`;
- `page-size` 5–100; таймауты положительны и `idle < max-age`;
- `trusted-proxies` — валидные CIDR;
- предупреждение (не ошибка), если `listen` не loopback и TLS не настроен;
- предупреждение, если `base-path` не `/` и `trusted-proxies` пуст за прокси.

Старые поля (`jackett`, `transmission`, `torrserver`, `telegram-api-token`, `users-list`,
`path`) не меняются; `jackett.timeout` уже поддерживается в `common.Settings`.

## 2. Генерация пароля

Пароль читается из stdin, чтобы не попасть в историю shell и `ps`:

```bash
read -rs PASSWORD && printf %s "$PASSWORD" | ./torrentino -hash-password
```

Вывод — одна строка для `web.users[].password-hash`. Флаг не требует `settings.json`
и не запускает бота. Хеширование: PBKDF2-HMAC-SHA256, 600 000 итераций (см. 06).
Для каждого пользователя хеш генерируется отдельно.

## 3. Первый запуск

1. Проверить DNS поддомена:
   ```bash
   dig +short torrentino.example.com @1.1.1.1
   ```
   Для `example.com` wildcard уже работает и возвращает тот же публичный IP,
   что и базовый домен; отдельная A-запись обычно не нужна.
2. Сгенерировать хеши паролей (выше), добавить всех пользователей.
3. Добавить блок `web` в `settings.json`: `enabled`, `listen: "127.0.0.1:8088"`,
   `base-path: "/"`, `secure-cookies: true`, `trusted-proxies`.
4. Проверить права: `chmod 600 settings.json` (в файле токен бота, ключ Jackett и хеши).
5. Добавить HTTP `server` (80) для ACME, выпустить сертификат поддомена, затем добавить
   HTTPS `server` (§5.2–5.3).
6. `sudo ./update-service.sh` (или `go build` и ручной рестарт).
7. Проверить локально: `curl -fsS http://127.0.0.1:8088/healthz` → `{"status":"ok"}`.
8. Открыть `https://torrentino.example.com/`, войти, проверить `/find`,
   `/downloads`, `/torrserver`; убедиться, что cookie host-only и `Path=/`.

Если порт занят или адрес/`base-path` некорректны, процесс завершится с понятной ошибкой
при старте — это конфигурационная ошибка, а не «веб тихо не поднялся».

## 4. systemd и сигналы

`torrentino.service` не меняется: `WorkingDirectory=/opt/torrentino`, `Restart=always`,
логи в `/var/log/torrentino.log`, `TimeoutStopSec=30`.

Что нужно поправить в коде (Phase 0):

- обрабатывать `SIGTERM` вместе с `SIGINT` (`signal.NotifyContext`);
- по сигналу: `http.Server.Shutdown` с таймаутом 10 с, затем отмена контекста бота;
- `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` — значения из 04;
- не писать в лог тела запросов и секреты.

`Restart=always` перезапустит процесс при панике; сессии при этом теряются, это ожидаемо.
При включённом web падение Telegram-части не должно ронять процесс (см. 03, §5.5):
ошибка инициализации бота логируется, веб продолжает работать в режиме «Telegram
недоступен» с баннером.

## 5. Сеть, TLS и nginx

### 5.1. Схема деплоя

```
браузер ──https://torrentino.example.com/──► nginx:443 (TLS)
                                                       │ server_name torrentino...
                                                       ▼
                                            torrentino 127.0.0.1:8088
                                            base-path=/
```

- Поддомен — отдельный origin: два других приложения на `example.com` не
  делят с нами origin, cookie и CSRF-токены. Риск T15 закрыт.
- TLS терминируется nginx; сертификат поддомена отдельный (текущий сертификат базового
  домена его не покрывает).
- Приложение не публикует 8088 наружу; порт слушает loopback.
- `base-path` в основном деплое — `/`; префикс в URL отсутствует.
- Старый путь `https://example.com/torrentino/` на время миграции отдаёт
  `301` на поддомен (см. §5.5).

### 5.2. Сертификат поддомена

Текущий сертификат `example.com` содержит SAN только для базового имени
(срок до 11.11.2026) и **не покрывает** `torrentino.example.com` — браузер
получит ошибку имени. Выпустите отдельный сертификат (не трогая два других приложения):

```bash
sudo mkdir -p /var/www/certbot
sudo certbot certonly --webroot -w /var/www/certbot \
  --cert-name torrentino.example.com \
  -d torrentino.example.com
```

Пути по умолчанию:

```
/etc/letsencrypt/live/torrentino.example.com/fullchain.pem
/etc/letsencrypt/live/torrentino.example.com/privkey.pem
```

HTTP-01 требует, чтобы 80/tcp доходил до nginx. Если 80 не проброшен — используйте
DNS-01 (`--dns-...` или ручной TXT). Wildcard-сертификат не обязателен: достаточно
одного имени. Не расширяйте существующий сертификат без нужды — отдельный обновляется
независимо.

Порядок: сначала добавьте HTTP `server` (80) из §5.3 и выполните `nginx -t`/reload,
затем выпустите сертификат, и только после этого добавляйте HTTPS-блок — иначе
`nginx -t` упадёт на ещё не существующих файлах `fullchain.pem`/`privkey.pem`.

### 5.3. nginx для поддомена (основной вариант)

HTTP-блок нужен для ACME и редиректа; HTTPS — отдельный `server` с именем поддомена.
Остальные приложения на `example.com` не меняются.

```nginx
server {
    listen 80;
    server_name torrentino.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    server_name torrentino.example.com;

    ssl_certificate     /etc/letsencrypt/live/torrentino.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/torrentino.example.com/privkey.pem;

    add_header Strict-Transport-Security "max-age=31536000" always;
    client_max_body_size 128k;

    location / {
        proxy_pass         http://127.0.0.1:8088;   # base-path=/, без URI
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;   # поиск Jackett до 60 с
        proxy_send_timeout 120s;
    }

    access_log /var/log/nginx/torrentino.access.log;
    error_log  /var/log/nginx/torrentino.error.log;
}
```

`proxy_pass` без URI сохраняет путь (`/find`, `/static/...`), а `base-path: "/"` означает,
что приложение ждёт эти пути в корне.

Проверка:

```bash
sudo nginx -t && sudo systemctl reload nginx

dig +short torrentino.example.com @1.1.1.1
openssl s_client -connect torrentino.example.com:443 \
  -servername torrentino.example.com </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -ext subjectAltName

curl -fsS https://torrentino.example.com/healthz
```

При прокси:

- `trusted-proxies` содержит адрес, с которого приходит nginx (`127.0.0.1/32` для
  того же хоста; адрес docker-сети, если nginx в контейнере);
- `secure-cookies: true`, иначе браузер не пришлёт cookie по HTTPS;
- таймауты прокси больше `jackett.timeout` (60 с), иначе поиск обрывается на nginx;
- HSTS: `includeSubDomains` не добавляем — wildcard DNS отдаёт любые поддомены, и
  политика может сломать посторонние имена. Если базовый домен уже отдаёт
  `includeSubDomains`, проверьте, что все используемые поддомены работают по HTTPS.

### 5.4. Fallback: путь `/torrentino`

Поддержка `web.base-path` сохранена, но это **не основной режим**: на одном origin с
двумя приложениями снова появляется риск T15. Если поддомен временно недоступен:

1. В `settings.json` поставить `base-path: "/torrentino"` и перезапустить сервис.
2. В `server` для `example.com` добавить:

```nginx
location = /torrentino {
    return 301 /torrentino/;
}

location /torrentino/ {
    proxy_pass         http://127.0.0.1:8088;   # без завершающего /
    proxy_http_version 1.1;
    proxy_set_header   Host              $host;
    proxy_set_header   X-Real-IP         $remote_addr;
    proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header   X-Forwarded-Proto $scheme;
    proxy_read_timeout 120s;
    proxy_send_timeout 120s;
    client_max_body_size 128k;
}
```

Cookie тогда получает `Path=/torrentino`; ссылки и редиректы тоже с префиксом.
`proxy_pass` без завершающего `/` обязателен: со слэшем nginx обрежет префикс, и
приложение ответит 404.

### 5.5. Редирект со старого пути

При основном варианте (поддомен, `base-path: "/"`) в существующем `server` для
`example.com` оставьте на время миграции:

```nginx
location = /torrentino {
    return 301 https://torrentino.example.com/;
}
location /torrentino/ {
    rewrite ^/torrentino/(.*)$ https://torrentino.example.com/$1 permanent;
}
```

Старые cookie с `Path=/torrentino` перестанут использоваться — пользователям нужно
войти заново (сессии и так in-memory). Через 1–2 недели `location /torrentino/` можно
убрать.

### 5.6. Порты и периметр

- 8088/tcp — только loopback (или сеть nginx), не пробрасывать наружу.
- 443/tcp — nginx с TLS; Torrentino — отдельный `server_name`.
- 9117 (Jackett), 9091 (Transmission), 8090 (Torrserver) по-прежнему не должны быть
  открыты наружу; веб-интерфейс не меняет эту модель.
- `listen: "0.0.0.0:8088"` — только отладочный режим в доверенной сети; IPv6 —
  `"[::]:8088"`.
- Для прямого доступа из LAN без nginx: `listen: "0.0.0.0:8088"`,
  `base-path: "/"` и понимание, что пароль идёт по HTTP.

## 6. Логи, health, мониторинг

- `GET {base}/healthz` (без авторизации) → `200 {"status":"ok"}`; в основном деплое —
  `https://torrentino.example.com/healthz`. Не содержит версий и конфигурации.
- Access log: мутации, ошибки 4xx/5xx и логин — всегда; успешные GET-страницы — на уровне
  debug (опрос `/downloads` каждые 5 с иначе засоряет лог).
- Формат: `web: ip=… user=… method=… path=… status=… dur=… request_id=…`.
- Аудит: login ok/fail, logout, delete (что/куда), torrserver delete, rate-limit.
- `/var/log/torrentino.log` растёт; настроить `logrotate` (ежедневно, 7–14 дней, `compress`)
  или перейти на journald (`StandardOutput=journal`).
- Мониторинг: health-чек + алерт на рестарты (`systemctl show -p NRestarts`).

## 7. Обновление и откат

Веб-часть не добавляет шагов к текущему процессу: статика и шаблоны встроены в бинарник.
Конфигурация nginx меняется один раз при включении веб-интерфейса.

```bash
sudo ./update-service.sh
```

- Новые поля `settings.json` необязательны: без блока `web` всё работает как раньше.
- Схемы/миграции БД нет.
- Откат: сохранить предыдущий бинарник (`/opt/torrentino/torrentino.prev`) и вернуть его;
  конфиг обратно совместим. `server`-блок поддомена можно оставить — без `web.enabled`
  он будет отдавать 502/404, пока сервис выключен.
- При обновлении Go пересобрать бинарник; для SSR-варианта больше нечего синхронизировать
  (для WASM понадобился бы `wasm_exec.js`, см. 07).

## 8. Резервное копирование

- `settings.json` — единственный файл с состоянием конфигурации (пароли в виде хешей).
  Бэкапить вместе с `path.*`-каталогами и торрент-клиентом; хранить с правами 600.
- Сессии/экраны не персистентны — бэкапить нечего.
- После восстановления на другом хосте пароли продолжат работать; сессии — нет.
- Конфиг nginx и сертификаты — в общем бэкапе; сертификат поддомена обновляется certbot.

## 9. Диагностика

| Симптом | Вероятная причина | Что проверить |
|---|---|---|
| Ошибка сертификата на поддомене | сертификат только для базового домена | выпустить сертификат на `torrentino.example.com`, `openssl s_client` |
| Поддомен открывает чужое приложение | нет `server_name`/`server`-блока | nginx `-T`, порядок `server`-блоков, SNI |
| 404 на `/find`, `/static/...` | приложение ждёт не тот путь | `base-path` (`/` для поддомена), `proxy_pass` |
| Ссылки ведут на `/torrentino/...` | остался старый `base-path` | `web.base-path: "/"`, перезапуск |
| Редирект на логин по кругу | `Secure` cookie/`X-Forwarded-Proto` | `secure-cookies`, `trusted-proxies`, TLS |
| 403 на всех POST | нет/просрочен CSRF, cookie не ставится | cookie/`Path`, часы, `Origin` |
| Cookie не отправляется | `Path` не совпадает | `Path=/` на поддомене, без `Domain` |
| «invalid username or password» при верном пароле | хеш сгенерирован другой версией/опечатка | перегенерировать, проверить формат |
| 502 на поиске | Jackett недоступен/неверный host:port/api-key | `curl` к Jackett, лог |
| Поиск обрывается через N секунд | таймаут прокси меньше `jackett.timeout` | `proxy_read_timeout`, `WriteTimeout` |
| Пустой футер загрузок | `Statfs` по `path.default` недоступен | путь существует и смонтирован |
| Нет автообновления | JS отключён/ошибка CSP | консоль браузера, `/static/app.js` |
| Не стартует после правки конфига | ошибка валидации web-полей | вывод процесса (fail-fast список) |
| 429 при входе | сработал rate limit | подождать, посмотреть лог, войти с другого IP |
| Диск не удаляется | путь вне разрешённых корней | `path.*`, лог аудита, 409 |
| Старый путь `/torrentino` не редиректит | нет `location`/`rewrite` | конфиг базового `server` |
| Из LAN поддомен не открывается | выключен hairpin NAT | локальная DNS-запись на `192.168.1.2` или проверка снаружи |

## 10. Чеклист эксплуатации

- [ ] `web.enabled: true`, все пользователи добавлены, `settings.json` 600.
- [ ] `listen: 127.0.0.1:8088`, `base-path: "/"`, `secure-cookies: true`.
- [ ] DNS `torrentino.example.com` резолвится в публичный IP.
- [ ] Отдельный сертификат поддомена выпущен и автоматически обновляется.
- [ ] nginx: HTTP→HTTPS, `server_name torrentino...`, `proxy_pass` без URI.
- [ ] Старый `/torrentino` на базовом домене отдаёт 301 на поддомен.
- [ ] 8088 недоступен из интернета; 443 — nginx.
- [ ] `/healthz` отвечает на поддомене и локально, логи не содержат секретов.
- [ ] Cookie: `Path=/`, `Secure`, `HttpOnly`, `SameSite=Lax`, без `Domain`.
- [ ] Автозапуск и рестарт (systemd) проверены, graceful shutdown работает.
- [ ] Резервная копия `settings.json`, nginx-конфига и сертификатов есть.
- [ ] Проверен сценарий «Telegram недоступен» (09).
