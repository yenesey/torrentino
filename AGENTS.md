# AGENTS.md

Краткое описание проекта для AI-агентов и разработчиков.

## Что это

`torrentino` — Go-приложение для управления «домашней» экосистемой загрузок на NAS.
Основной интерфейс — Telegram-бот; при `web.enabled: true` тот же бинарник поднимает
веб-интерфейс в браузере. Оба интерфейса дают поиск раздач, постановку загрузок,
просмотр/управление списком загрузок и мгновенный просмотр видео через Torrserver.

Внешние интеграции:
- **Jackett** — метапоисковик торрентов (API v2.0), используется как поисковый движок.
- **Transmission** — торрент-клиент: добавление, пауза/старт, удаление, список.
- **Torrserver** — стриминг видео без полной загрузки (add/list/rem).

Репозиторий: `github.com/yenesey/torrentino`. Модуль: `torrentino`. Go 1.27 (`toolchain go1.27.2`).
Объём — несколько тысяч строк Go (Telegram-часть, общий слой `core`/`common/list`,
веб-интерфейс `web`).

Тесты: `common/settings_test.go`, `common/list/list_test.go`, `common/password/password_test.go`,
`common/paginator/paginator_test.go`, `core/core_test.go`, `core/presentation/presentation_test.go`,
`api/torrserver/torrserver_test.go`,
`web/*_test.go` (включая интеграционные через `httptest`).

## Сборка и запуск

```bash
go build ./...          # сборка (проверено: OK)
go vet ./...            # статический анализ (проверено: чисто)
go test -race ./...     # тесты (проходят; web-тесты медленные из-за PBKDF2)
go build -v -ldflags "-s -w"   # релизная сборка (как в update-service.sh)
```

Запуск — **только из каталога, где лежит `settings.json`**: конфиг читается по
относительному пути `./settings.json`, путь не переопределяется ни флагом, ни переменной
окружения. Загрузка выполняется явным вызовом `common.MustLoadSettings()` в начале
`main()`, а не из `init()` — см. п. 1 подводных камней.

```bash
./torrentino
```

Для веб-аккаунтов хеш пароля генерируется флагом (пароль читается из stdin, `settings.json`
при этом не нужен):

```bash
read -rs PASSWORD && printf %s "$PASSWORD" | ./torrentino -hash-password
```

Если конфига нет или он невалиден, бот не стартует: печатает диагностику и выходит
с кодом 1 (см. п. 1 подводных камней).

Линтер-конфигов в проекте нет. Тесты покрывают конфиг (включая блок `web`), `common/list`,
пароли, разбор callback-данных и web-команд, домен `core` (с фейками) и web-интерфейс
(интеграционные через `httptest`). Telegram-сценарий по-прежнему проверяется вручную.

## Структура

```
main.go                     точка входа: -hash-password, web (до бота), бот, SIGTERM/shutdown
settings.json               конфиг (в .gitignore, в репозиторий не коммитится)
common/
  settings.go               LoadSettings/MustLoadSettings: чтение, валидация, common.Settings (в т.ч. web)
  settings_test.go          тесты загрузки и валидации конфига (через LoadSettingsFrom)
  basepath.go               NormalizeBasePath для web.base-path
  password/password.go      PBKDF2-HMAC-SHA256 хеши/проверка, формат для settings.json
  password/password_test.go тесты формата, соли и проверки пароля
  utils/utils.go            FormatFileSize, ReadDir (канал), WithTimeout, LogError
  ordmap/ordmap.go          generic-ordered map с iter.Seq2 (итераторы Go 1.23+)
  list/list.go              UI-агностичный List: индексы, Evaluator, Sorting, фильтры, очередь
  list/view.go              list.View: страница, выбор, Prepare, Select, Toggle*, ExecuteSelected
  list/list_test.go         тесты границ, сортировок, фильтров и ExecuteSelected
  paginator/paginator.go    Telegram-рендерер: сообщение + inline-клавиатура, dispatch
  paginator/paginator_test.go тесты разбора callback-данных (через dispatch, без бота)
  paginator/list.go         алиасы List/Sorting/Evaluator на common/list
core/
  actions.go                Action, sentinel-ошибки, ActionIDs
  services.go               Service и интерфейсы JackettAPI/TransmissionAPI/TorrserverAPI + адаптеры
  search.go                 FindItem, Search/ResolveHash/FindActions/DoFind/TorrentFile/Poster
  downloads.go              DownloadItem, Downloads (Transmission + диск), сортировки, действия, пути
  torrserver.go             TorrserverItem, список/сортировка/delete
  screens.go                общие дескрипторы сортировок и фильтров экранов
  presentation/             общие вычисляемые значения строк и итогов загрузок
  core_test.go              юнит-тесты домена с фейками
web/
  server.go, middleware.go  HTTP-сервер, маршруты, base-path, логи, заголовки, auth/CSRF middleware
  auth.go, sessions.go      логин/логаут, PBKDF2, серверные сессии, cookie, rate limit
  csrf.go, limiter.go       CSRF/Origin, лимит попыток логина
  screens.go, commands.go   состояние экрана на сессию, парсер web-команд, действия
  screen_find.go, screen_downloads.go, screen_torrserver.go, torrent.go, view.go, render.go
  downloads_cache.go        снимок /downloads с TTL 2 с
  templates/, static/       встроенные html/template, CSS, app.js, htmx.min.js
api/
  jackett/jackett.go        поиск, парсинг .torrent для InfoHash, cookiejar
  transmission/transmission.go  тонкие обёртки над transmissionrpc/v2 (клиент через sync.Once)
  torrserver/torrserver.go  JSON-over-HTTP POST /torrents, помощник post()
  torrserver/torrserver_test.go тесты экранирования, инъекции и разбора ответа
telegram/
  search/search.go          Telegram-рендер поиска; домен — core.Service
  downloads/downloads.go    Telegram-рендер списка загрузок, автообновление раз в 5 с
  torrserver/torrserver.go  Telegram-рендер Torrserver
torrentino.service          systemd-юнит (WorkingDirectory=/opt/torrentino)
update-service.sh           сборка → stop → mv в /opt/torrentino → start (нужен root)
```

## Архитектура

**Общий слой.** `common/list` — UI-агностичное состояние списка (`List`, `View`,
`Sorting`, `Evaluator`, `Select`/`NextPage`/`ToggleSorting`/`ToggleFilter`/`ExecuteSelected`).
`core` — домен: элементы (`FindItem`, `DownloadItem`, `TorrserverItem`), `Service`,
`AvailableActions`, `Do*`, `TorrentFile`, обход диска и дескрипторы сортировок/фильтров
(`FindSorting`, `DownloadsFilters`, …). `core/presentation` вычисляет общие для Telegram
и web значения строк (размеры, проценты, статусы, итоги загрузок), а каждый интерфейс
добавляет только свой markup. `web` не импортирует `telegram` и `api/*` напрямую.

**Паттерн экрана.** Каждый Telegram-экран — структура, встраивающая `paginator.Paginator`, и
переопределяющая методы через интерфейсы:
- `Builder`: `Header` / `Footer` / `Line(i)` — текст;
- `Actor`: `Actions(i) []string` / `Execute(i, action) bool` — кнопки и их обработка;
- `Evaluator`: `Compare(i, j, attr) bool` / `Stringify(i, attr) string` — сортировка и фильтры.

Плюс собственный `Reload() error`, который наполняет список (`Alloc` + `Append`).
«Перегрузка» методов эмулируется встраиванием и повторным объявлением с тем же именем.

**Обработка нажатий.** Все callback-и одного экрана регистрируются на общий префикс
(`find`, `list`, `torrserver`) через `bot.RegisterHandler(..., MatchTypePrefix, ...)`.
Реестр `paginator.Handlers` хранит `prefix → handler id`; при создании нового пагинатора
с тем же префиксом предыдущий handler снимается. Callback-данные кодируются как
`prefix + команда`, где команды — константы `CB_ORDER_BY`, `CB_FILTER_BY`, `CB_ACTION`,
`CB_NEXT_PAGE`, `CB_PREV_PAGE`, `CB_TOGGLE_FILTERS`, `CB_STUB`.

**Рендер.** `Paginator.Show()` пересобирает текст и клавиатуру и редактирует сообщение
только если текст или клавиатура реально изменились (`EditMessageText` /
`EditMessageReplyMarkup`), чтобы не упираться в лимиты Telegram.

**Доступ.** Telegram: middleware в `main.go` пропускает только `From.ID` из `users-list`;
чужие сообщения просто логируются. Первый запуск с пустым `users-list` — способ узнать
свой ID из логов. Web: собственные аккаунты `web.users` с PBKDF2-хешами, серверные сессии
в памяти, CSRF + проверка `Origin`, rate limit на логин; `users-list` для web не
используется, поэтому вход работает при недоступном Telegram.

**Web.** `web.Server` поднимается из `main()` до Telegram, если `web.enabled`. При
`base-path: "/"` URL без префикса; при `/torrentino` все ссылки, формы, редиректы, статика
и cookie получают префикс. SSR: `html/template` + встроенные `web/static/{app.css,app.js,
htmx.min.js}`. Все действия — POST с CSRF; GET ничего не меняет. Состояние экранов живёт
в сессии (по одному `find`/`downloads`/`torrserver`), выбор хранится по стабильному ключу
(`id:`, `path:`, `hash:`), а `/downloads` опрашивается раз в 5 с (пауза при скрытой
вкладке, серверный кэш снимка 2 с).

## Соглашения

- Отступы — табы (`.editorconfig`, `indent_size = 4`).
- Один Telegram-экран — один пакет в `telegram/`, внешний сервис — один пакет в `api/`.
- Общие вещи — в `common/` или `core/`; UI-агностичное состояние списка — в `common/list`.
- Вычисляемые значения для отображения — в `core/presentation`; HTML/Telegram-markup — только
  в `web`/`telegram`.
- Ошибки оборачиваются `github.com/pkg/errors` и логируются через `utils.LogError`
  (печатает файл, строку и имя функции). Исключение — `common/settings.go`: там
  используется stdlib `errors` / `fmt.Errorf` с `%w`, а конфигурационные ошибки
  фатальны и намеренно не проходят через `utils.LogError`.
- Внешние вызовы ограничены таймаутами и контекстом: Jackett/Transmission/Torrserver
  принимают `context.Context` (`*Context`-варианты), `.torrent` — 30 с/25 МБ, постер —
  10 с/2 МБ, действия web — 20 с. Таймаут поиска Jackett (60 с по умолчанию) берётся из
  `common.Settings.JackettTimeout()` — это `jackett.timeout` в конфиге, строка-длительность
  (`"60s"`); пустое значение даёт 60 с, неразборчивое ловится валидацией при загрузке.
  `utils.WithTimeout` остаётся в `common/utils`, но новый код должен использовать контекст,
  чтобы отмена реально прерывала запрос, а не оставляла висеть горутину.
- Тела запросов к внешним сервисам собираются через `json.Marshal` по структуре
  (`addRequest` / `listRequest` / `remRequest`), а не конкатенацией строк: в поля
  попадают недоверенные данные — названия раздач с трекеров и постеры, выскребенные
  из HTML чужих страниц.
- Пакеты `api/*` читают `common.Settings` **во время вызова**, а не в `init()`: конфиг
  загружается в `main()`, поэтому всё, что захвачено при инициализации пакета, осталось
  бы пустым.
- UI-текст — HTML (`ParseModeHTML`), с emoji-индикаторами: 📥 в Transmission,
  🎦 в Torrserver, 🧲 magnet, 📎 .torrent, 🎬/🎧 по расширению.
- Комментарии в коде — смесь английского и русского.
- Правила импортов: `web` → `core`, `core/presentation`, `common`, stdlib (не
  `telegram`/`api/*`); `telegram` → `core`, `core/presentation`, `common/paginator`,
  `common`; `core` → `api/*`, `common`, `common/list`; `core/presentation` → `core`,
  `common/utils`; `common/list` → stdlib, `common/ordmap`, `common/utils`; `api/*` → `common`.

## Подводные камни

Стоит знать до правок:

1. **Путь к конфигу.** `./settings.json` — относительно CWD и не переопределяется
   ни флагом, ни переменной окружения, поэтому запускать бота нужно из каталога с
   конфигом. Ошибка обрабатывается fail-fast: `common.MustLoadSettings()` вызывается
   первым делом в `main()`, печатает абсолютный путь, текущий каталог, а для
   битого/неполного файла — список всех проблемных полей сразу, и выходит с кодом 1.
   Обязательными считаются `telegram-api-token`, `jackett.api-key` и `host`/`port`
   всех трёх сервисов; пустые `users-list` и `path.*` дают только предупреждение в лог.
   Раньше при отсутствии файла процесс продолжал работу с нулевым `common.Settings`
   и URL вида `http://:9117/`.
   Загрузка намеренно **не** делается в `init()`: `go test` запускает бинарник с рабочим
   каталогом пакета, где `settings.json` нет, и `log.Fatalf` из `init()` убивал бы любой
   тест, импортирующий `common`. Отсюда же требование к `api/*` не читать `Settings`
   при инициализации пакета (см. соглашения). Для тестов есть `LoadSettingsFrom(path)`.
2. **`settings.json` в рабочем дереве содержит реальные секреты** (токен бота, список
   user id). Файл в `.gitignore` и не отслеживается git — не коммитить его и не копировать
   значения из него в код, документацию и ответы.
3. **Опечатки в ключах `settings.json` молча игнорируются:** `json.Unmarshal` вызывается
   без `DisallowUnknownFields`, поэтому неизвестный ключ просто не заполняет поле и не
   даёт ошибки. Пример конфига в `readme.md` приведён в соответствие с кодом и проверен
   прогоном через реальный парсер (объект `path` с `default`/`movie`/`series`; раньше там
   фигурировал `download_dir`, который ни на что не влиял). Обязательные поля теперь
   ловятся валидацией, но опечатка в `path.*` даёт только предупреждение в лог.
   `jackett.timeout` и `web.session.idle-timeout`/`max-age` — строки-длительности
   (`"60s"`, `"12h"`); число без единицы (`60`) считается ошибкой, чтобы «60» не
   читалось как 60 наносекунд.
4. **Глобальное состояние.** `common.Settings`, реестр `paginator.Handlers` и
   `downloads.Updater` — пакетные/глобальные. Один живой экран на префикс **на процесс**,
   поэтому одновременные пользователи в разных чатах перетирают друг другу одно и то же
   сообщение. `updater` в `downloads` тоже один: новый `/downloads` отменяет предыдущий.
5. **Автообновление `/downloads`** — горутина с тикером 5 с, перерисовывает сообщение
   каждые 5 секунд, пока не отменён контекст.
6. **Удаление файлов.** В `telegram/downloads` для позиций со статусом `unknown`
   (файлы на диске, которых нет в Transmission) действие `delete` вызывает
   `os.Remove`/`os.RemoveAll` напрямую — это необратимо.
7. **Разбор callback-данных** (исправлено в `Paginator.dispatch`, есть тесты в
   `paginator_test.go`). Прежний `callbackHandler` падал на данных, которые Telegram
   вполне может доставить: `cmd[0]` без проверки длины — паника на пустом callback,
   `strings.Split(payload, "/")` + `split[1]` — паника на команде фильтра без
   разделителя, `p.selectedItem` без границ — паника в `Actor.Execute`, если список
   успел сократиться, и `unicode.IsNumber(rune(cmd[0]))` с `strconv.Atoi` без проверки
   ошибки — «12abc» молча выбирал элемент 0. Сейчас: `parseItemIndex` принимает только
   ASCII-цифры и проверяет переполнение, индекс сверяется с `Len()`, команды с payload
   разбираются через `strings.CutPrefix` / `strings.Cut`, а индекс проверяется ещё раз
   перед `Execute`. Паника в горутине обработчика убивает весь процесс, а systemd
   (`Restart=always`) превращает это в цикл перезапусков — поэтому здесь не должно быть
   ни одного индексирования без проверки.
   `Paginator.lastPage()` — единственный источник истины о последней странице: раньше
   кнопка «➡» и её обработчик считали её разными формулами, и нажатие могло увести на
   пустую страницу. Тесты дёргают `dispatch` напрямую, без бота и сети.
8. **Transmission:** учётные данные захардкожены как `rpcuser`/`rpcpass`
   (`api/transmission/transmission.go`), в коде есть закомментированный `todo` про
   `transmissionrpc/v3`. RPC-клиент создаётся лениво через `sync.Once` и запоминает хост
   при первом обращении, поэтому тест, меняющий `common.Settings.Transmission` после
   первого вызова, продолжит ходить на старый адрес.
9. **Torrserver.** Раньше тела запросов склеивались из строк без экранирования: название
   раздачи с кавычками или переводом строки давало невалидный JSON (действие молча
   падало с `request error` в логе), а подделанное название могло дописать в запрос
   чужие `link`/`action`/`hash` — название приходит из раздачи на трекере, то есть
   недоверенное. Сейчас всё сериализуется через `json.Marshal` (`addRequest`,
   `listRequest`, `remRequest`), а единый помощник `post()` в `api/torrserver` отвечает
   за POST, дедлайн, `defer res.Body.Close()` и разбор ответа; сломанная функция
   `httPost` удалена. На это есть тесты в `api/torrserver/torrserver_test.go`.
   Отдельно: поле `data` в ответе — «произвольные данные», поэтому его разбор
   в `DataStruct` сделан необязательным и не роняет весь список.
   `api/torrserver`, `api/jackett` и `api/transmission` теперь принимают контекст
   (`*Context`-варианты); старые обёртки оставлены для совместимости. Скрейпинг постера
   и `.torrent` вынесены в `core` с таймаутами и лимитами.
10. **Только Unix:** `syscall.Statfs` в футере `/downloads` делает сборку под Windows
    невозможной без правок.
11. **`utils.LogError`** предполагает путь пакета с префиксом (`parts[0][11:]`) — вызов
    из нестандартного окружения может дать мусорное имя.
12. **Мёртвый код в `api/jackett`:** `GetValidIndexers` никто не вызывает, а его запрос
    был относительным URL (`indexers?Configured=true`) и всё равно не сработал бы;
    сейчас URL достроен до абсолютного. Мораль та же, что с `httPost`: неиспользуемые
    функции `go vet` не помечает, поэтому их надо искать глазами или через staticcheck
    (U1000).
13. **Web-сессии и состояние.** Сессии только в памяти: рестарт процесса = разлогин,
    экраны пропадают. Один экран каждого типа на сессию (вкладки одного типа делят
    состояние). Несколько web-аккаунтов равноправны; ролей/read-only в v1 нет.
14. **Web `base-path` и TLS.** `web.base-path` должен быть `/` или путём без `//`, `..`,
    query/fragment; при fallback все URL и cookie идут с префиксом. За TLS-прокси нужны
    `secure-cookies: true` и `trusted-proxies`, иначе `Origin`/`X-Forwarded-Proto`
    проверки и `Secure` cookie работают неправильно. Приложение по умолчанию слушает
    loopback; менять на `0.0.0.0` без TLS — только отладочный режим.
15. **Web-удаление необратимо** (паритет с Telegram): POST + CSRF + проверка `Origin`,
    выбор по стабильному ключу, для файлов на диске — проверка `Clean` внутри
    `path.default/movie/series`. Отдельного подтверждения нет; аудит пишется в лог.
16. **Web-кэш `/downloads`** — снимок с TTL 2 с, чтобы несколько вкладок не дёргали
    Transmission и диск на каждый опрос. После действия кэш инвалидируется; ошибка
    upstream сохраняет старый список и показывает баннер.
17. **`-hash-password`** читает пароль из stdin и не загружает `settings.json`; не путать
    с `telegram-api-token`. Пароли хранятся только хешами PBKDF2 (100k–2M итераций,
    по умолчанию 600k).
18. **Callback-запросы обходят ACL (не исправлено).** Middleware доступа в `main.go`
    проверяет `users-list` только внутри `if update.Message != nil`, а у нажатия на
    кнопку `update.Message == nil`, поэтому проверка пропускается и
    `Paginator.callbackHandler` выполняется для любого нажатия. Получить клавиатуру
    посторонний не может (бот шлёт сообщения только в разрешённые чаты), но у **удалённого
    из `users-list` пользователя** старые сообщения с кнопками остаются живыми, и он
    продолжает управлять ботом — а состояние экранов глобальное (п. 4), так что это
    затрагивает и остальных. Лечится распространением той же проверки на
    `update.CallbackQuery.From.ID`; сделать это стоит осознанно, потому что это меняет
    модель доступа (например, в групповом чате жать кнопки сможет только тот, кто есть
    в `users-list`).

## Деплой

`torrentino.service` ожидает бинарник в `/opt/torrentino`, логи пишутся в
`/var/log/torrentino.log`, `Restart=always`. Обновление — `sudo ./update-service.sh`
(собирает с `-ldflags "-s -w"`, останавливает сервис, подменяет бинарник, запускает).

Веб-деплой: `https://torrentino.example.com/`, nginx на 443,
приложение на `127.0.0.1:8088` (`web.base-path: "/"`, `secure-cookies: true`,
`trusted-proxies: ["127.0.0.1/32"]`). Сайт nginx —
`/etc/nginx/sites-available/torrentino` (шаблон в `deploy/nginx/torrentino.conf`),
сертификат — `/etc/letsencrypt/live/torrentino.example.com/` через
`certbot --webroot -w /var/www/certbot`; старый путь `/torrentino` на базовом домене
редиректится на поддомен. Проверки: `nginx -t`, `certbot renew --dry-run`,
`curl https://torrentino.example.com/healthz`.

`main.go` оборачивает стандартный `log` redacting-writer'ом: библиотека Telegram
пишет URL с токеном бота при сетевых ошибках, и токен не должен попадать в
`/var/log/torrentino.log`. Если старые логи содержали токен, его нужно отозвать
через BotFather и заменить в `settings.json`.
