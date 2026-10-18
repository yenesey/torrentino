# 03. Архитектура

## 1. Как устроено сейчас

```
main.go
 ├── ACL-middleware (users-list)
 ├── default handler ──► telegram/search     ─┐
 ├── /downloads ───────► telegram/downloads   ├──► api/jackett
 └── /torrserver ──────► telegram/torrserver  ├──► api/transmission
                                             └──► api/torrserver
common/paginator: Telegram-сообщение + inline-клавиатура + List (сортировки/фильтры)
common/settings, common/utils
```

Особенности, важные для второго UI:

- `telegram/*` одновременно содержат UI (форматирование строк, `Builder/Actor/Evaluator`)
  и доменную логику (вызовы `api/*`, обход диска, `Statfs`, скрейпинг постера).
- `common/paginator/paginator.go` жёстко привязан к Telegram (`bot.Bot`, `models.Update`,
  callback-данные, `EditMessageText`). При этом `common/paginator/list.go` — чистый,
  UI-агностичный код: `List`, `Sorting`, `Evaluator` не импортируют Telegram.
- Логика `dispatch` (границы индексов, сброс страницы, мультисортировка) — самое тонкое
  место: в ней уже были паники и расхождения `lastPage` (AGENTS.md, п. 7).
- Состояние экранов глобальное: `paginator.Handlers` — один handler на префикс на процесс.
- `api/jackett` и `api/torrserver` читают `common.Settings` во время вызова (это правильно),
  но не принимают контекст; `api/transmission` использует `context.TODO()`.
- В `telegram/search` есть сетевые вызовы внутри «геттеров»: `Actions(i)` скачивает
  `.torrent` для InfoHash, `Execute` скрейпит страницу постера без таймаута
  (`htmlquery.LoadURL`), а `.torrent` качается `http.Get` без таймаута и не закрывает body.

## 2. Что мешает просто «добавить HTTP»

Если веб-обработчики начнут повторно вызывать `api/*` и заново реализовывать сортировки,
фильтры, доступность действий и границы страниц, поведение двух UI разъедется. Это
гарантированный источник багов: правила нетривиальны (мультисортировка, «все фильтры
выключены = показать всё», сброс страницы при фильтре, `unselect` после действия).

Поэтому проект делится на два слоя, общих для Telegram и web:

1. **`core/`** — доменные операции и элементы списков: что можно сделать с элементом,
   как его сравнивать/фильтровать, какие действия доступны. Без Telegram и без HTML.
2. **`common/list`** — UI-агностичный список и состояние просмотра: элементы, сортировка,
   фильтры, страница, выбор, `Dispatch`-операции. Telegram и web только рендерят.

## 3. Целевая слоистость

```
                         main.go
                        /       \
             telegram/* (Telegram)  web/* (HTTP)
                     \               /
                      \             /
                       core/  (Service, Item, Action)
                         |
                  api/{jackett,transmission,torrserver}
                         |
              Jackett / Transmission / Torrserver

common/list   — List, View, Sorting, Evaluator (без bot и без net/http)
common/*      — Settings, utils, ordmap
```

Правила импортов (проверяются ревью и, позже, `go vet`-линтером/скриптом):

| Пакет | Может импортировать | Не может |
|---|---|---|
| `web` | `core`, `core/presentation`, `common`, stdlib | `telegram`, `api/*` напрямую, `go-telegram/*` |
| `telegram` | `core`, `core/presentation`, `common/paginator`, `common` | `web`, `api/*` напрямую (после Phase 0) |
| `core` | `api/*`, `common`, `common/list` | `web`, `telegram`, `core/presentation`, `common/paginator`, Telegram |
| `core/presentation` | `core`, `common/utils` | `web`, `telegram`, `api/*` |
| `common/list` | stdlib, `common/ordmap`, `common/utils`, `pkg/errors` | всё остальное |
| `common/paginator` | `common/list`, Telegram | `core`, `web` |
| `api/*` | `common` | `core`, `web`, `telegram` |

`common.Settings` и клиенты `api/*` остаются глобальными, как сейчас: конфиг читается
один раз в `main()`, поэтому `api/*` обязаны читать настройки во время вызова
(существующее соглашение сохраняется).

## 4. Phase 0 — рефакторинг без изменения поведения

Это обязательный фундамент: без него веб начнёт дублировать логику. Все шаги выполняются
до появления первого HTTP-обработчика; критерий — Telegram работает как раньше.

### 4.1. Вынести `List`/`View` в `common/list`

- Перенести из `common/paginator` типы `List`, `Sorting`, `Evaluator` и метод `Delete`,
  `Alloc`, `Append`, `Filter`, `Sort`, `ToggleSorting`, `ToggleFilter`, `Setup*`.
  Код переносится почти дословно (импортов Telegram там нет).
- Добавить `View`:

  ```go
  type View struct {
      List
      Actor
      PerPage, Page, Selected int // Selected = -1 — ничего не выбрано
      Controls                bool // панель сортировок/фильтров
  }

  func (v *View) Bounds() (from, to int) // как pageBounds
  func (v *View) LastPage() int
  func (v *View) Prepare()               // Filter + Sort
  func (v *View) Select(i int) bool
  func (v *View) NextPage() bool
  func (v *View) PrevPage() bool
  func (v *View) ToggleSorting(attr string)
  func (v *View) ToggleFilter(attr, value string) // со сбросом страницы/выбора
  func (v *View) ToggleControls()
  func (v *View) ExecuteSelected(action string) bool // проверяет границы, зовёт Actor
  ```

  Semantics ровно как в `Paginator.dispatch`: `Select` принимает только `0 <= i < Len`,
  фильтр сбрасывает `Page` и `Selected`, сортировка снимает выбор, `ExecuteSelected`
  не вызывает `Actor`, если выбор вне границ. `list.Actor` — минимальный интерфейс
  `Execute(i int, action string) bool`: список доступных действий даёт `core`, подписи
  для кнопок — UI.
- `Paginator` встраивает `list.View`; `dispatch` остаётся в `paginator` как парсер
  Telegram callback-данных и вызывает методы `View`. Существующие тесты `dispatch`
  остаются; добавляются тесты `View` на границы и переходы.
- Хендлеры меняют `paginator.Sorting` на `list.Sorting` (или получают alias в
  `paginator` на время миграции).

Результат: `View` можно прогонять в юнит-тестах без Telegram и использовать в web.

### 4.2. Вынести домен в `core/`

Один пакет `core` (файлы `actions.go`, `search.go`, `downloads.go`, `torrserver.go`,
`services.go`):

```go
type Action struct {
    ID    string // "download", "download:series", "pause", "delete", ...
    Label string // подпись для обоих UI
}

type FindItem struct {
    jackett.Result
    InTorrents, InTorrserver bool
}
func (i *FindItem) Compare(a, b int, attr string) bool   // Evaluator
func (i *FindItem) Stringify(a int, attr string) string

type Service struct {
    Jackett      JackettAPI
    Transmission TransmissionAPI
    Torrserver   TorrserverAPI
}
func (s *Service) Search(ctx context.Context, query string) ([]*FindItem, error)
func (s *Service) ResolveHash(ctx context.Context, item *FindItem) error
func (s *Service) AvailableActions(item *FindItem) []Action
func (s *Service) Do(ctx context.Context, item *FindItem, action string) error
```

Аналогично `DownloadItem`/`TorrserverItem` со своими `AvailableActions`/`Do`, плюс:

- `Downloads(ctx) ([]*DownloadItem, DiskStats, error)` — Transmission + обход трёх
  каталогов (код из `downloads.Reload` переносится, включая иконки и `Statfs`);
- `TorrserverList`, `TorrserverDelete`.

Интерфейсы `JackettAPI`, `TransmissionAPI`, `TorrserverAPI` — узкие, по методам,
которые реально нужны `core`. Реализации по умолчанию — тонкие адаптеры над `api/*`.
Это даёт веб-тестам фейки без сети и без глобального `sync.Once` в Transmission.

### 4.3. Привести `api/*` к контексту и таймаутам

- `jackett.QueryContext(ctx, query, indexers)`, `GetInfoHashContext(ctx, link)`;
  старые обёртки можно удалить после перевода хендлеров.
- `transmission` методы принимают `ctx` (`TorrentGetAll(ctx)`, `TorrentAdd(ctx, ...)`).
- `torrserver.post` принимает родительский `ctx` (сейчас создаёт свой от `Background`).
- Скрейпинг постера: вместо `htmlquery.LoadURL` — HTTP-запрос с контекстом и таймаутом
  10 с, затем `htmlquery.Parse`; ошибка не блокирует добавление в Torrserver.
- `.torrent` для Telegram и web — один `core.TorrentFile(ctx, item)`, который закрывает
  body, ограничивает размер и проверяет, что тело похоже на bencode (первый байт `d`).

Это исправляет существующие дефекты (незакрытый body, зависание скрейпинга), но
не меняет пользовательские сценарии.

### 4.4. Хендлеры становятся рендерерами

`telegram/search`, `telegram/downloads`, `telegram/torrserver`:

- `Reload` вызывает `core.Service`;
- `Line`/`Header`/`Footer` остаются Telegram-форматированием;
- `Actions` берёт `core.AvailableActions` (после `ResolveHash` при выборе);
- `Execute` вызывает `core.Do` и переводит ошибку в лог/`ReplyMessage`;
- настройки сортировок/фильтров берутся из `core`-дескрипторов, чтобы web и Telegram
  не разъехались:

  ```go
  // core/screens.go
  func FindSorting() []list.Sorting
  func FindFilters() []string
  func DownloadsSorting() []list.Sorting
  func DownloadsFilters() []string
  func TorrserverSorting() []list.Sorting
  ```

  Размер страницы остаётся параметром UI: Telegram — 4, web — из конфига.

### 4.5. Жизненный цикл в `main.go`

- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` — сейчас обрабатывается
  только `os.Interrupt`, а systemd останавливает процесс SIGTERM. Без этого
  `http.Server.Shutdown` не сработает.
- Запуск web-сервера до бота (см. 5). Ошибка инициализации бота при включённом web —
  не фатальна: логируем и работаем в web-only режиме с баннером. При выключенном web
  поведение прежнее (`log.Fatal`).
- По сигналу: `web.Shutdown(ctxTimeout)` (10 с), затем отмена общего контекста бота.

## 5. Веб-сервер

### 5.1. Пакет `web/`

```
web/
  server.go        // NewServer, routes, http.Server, Start/Shutdown
  basepath.go      // нормализация base-path, url() helper, StripPrefix
  middleware.go    // request-id, access log, security headers, auth, CSRF, rate limit
  auth.go          // GET/POST /login, POST /logout, verify password
  sessions.go      // Store, Session, cookie, TTL/GC
  csrf.go          // токены, проверка Origin
  screens.go       // Kind, Screen, получение/создание, общий рендер
  screen_find.go
  screen_downloads.go
  screen_torrserver.go
  commands.go      // разбор web-команд и вызовы View
  torrent.go       // GET /find/torrent
  health.go
  assets.go        // go:embed templates/* static/*
  templates/*.html
  static/{app.css,app.js,htmx.min.js}
```

### 5.2. Состояние

```go
type Kind string // "find" | "downloads" | "torrserver"

type Session struct {
    mu       sync.Mutex
    User     string
    CSRF     string
    Created  time.Time
    LastSeen time.Time
    Screens  map[Kind]*Screen
}

type Screen struct {
    mu          sync.Mutex
    Kind        Kind
    View        list.View      // элементы + сортировка + фильтры + страница + выбор
    Query       string         // только find
    SelectedKey string         // стабильный ключ выбранного элемента
    Err         error          // последняя ошибка для баннера
    Updated     time.Time
    Searching   bool
    gen         uint64         // защита от применения устаревшего поиска/обновления
}
```

- Один экран каждого типа на сессию. Разные пользователи/сессии независимы.
- `View.Actor` — реализация `Screen`, вызывающая `core.Do` для выбранного элемента.
- `SelectedKey` — `id:<transmission-id>`, `path:<путь>` или `hash:<hash>`. Индекс в
  `View` используется для рендера, но при перезагрузке списка выбор переносится на
  элемент с тем же ключом; если элемент исчез — выбор снимается. Так автообновление
  не может перенаправить действие на соседнюю строку.
- Сессии — in-memory: рестарт процесса = разлогин. Это осознанно (см. 11, D4).
- **Не используем** `paginator.Handlers`, `Paginator` и Telegram-типы в web.

### 5.3. Потоки запросов

**Логин.** `GET {base}/login` → форма + pre-auth CSRF → `POST {base}/login` → проверка
пароля (PBKDF2, constant time) → новая сессия (новый токен, ротация) → `303 next|/find`
(в fallback — `/torrentino/find`).

**Поиск.** `POST {base}/find/search` (htmx) или обычная форма:
1. под `mu` проверяем запрос, ставим `Searching=true`, увеличиваем `gen`, копируем gen;
2. вне lock: `core.Search(ctx, q)` (ctx от запроса, дедлайн из `jackett.timeout`);
3. под lock: если `gen` не изменился — заменяем `View` (сортировки/фильтры по умолчанию,
   `Page=0`, `Selected=-1`, `SelectedKey=""`), иначе результат отбрасываем;
4. рендерим фрагмент/страницу.

**Команда.** `POST {base}/ui/{kind}/command`: парсим `cmd`, под lock применяем к `View`
(`Select`, `NextPage`, `ToggleSorting`, `ToggleFilter`, `ExecuteSelected`). Действия,
которые ходят в сеть/на диск, выполняются **вне** lock: сначала фиксируем выбранный
элемент, его стабильный ключ и `gen`, затем `core.Do`, затем применяем результат (флаг
`📥`, удаление элемента, ошибку) под lock, если `gen` не изменился.

**Удаление.** `action:delete` выполняется сразу, как в Telegram. Сервер берёт выбранный
элемент по `SelectedKey` и проверяет, что он всё ещё есть в текущем списке (а не только
индекс); если элемент исчез — `409` и «список изменился», удаления нет. Отдельного
подтверждения нет; защита — аутентификация, CSRF, проверка пути и аудит.

**Загрузки.** `GET {base}/downloads` (полная страница или фрагмент для опроса):
1. взять снимок из общего кэша (TTL 2 с) или перечитать Transmission + диск вне lock;
2. под lock аккуратно применить: сохранить сортировки/фильтры/страницу; пересобрать
   опции фильтров по фактическим значениям; зажать страницу; выбор перенести на элемент
   с тем же `SelectedKey`, а если его больше нет — снять выбор;
3. рендер.

**Torrent.** `GET {base}/find/torrent?item=N`: под lock проверяем границы и берём копию
`Link/Title`, вне lock — `core.TorrentFile(ctx, item)`, затем поток с
`Content-Disposition` и лимитом 25 МБ.

**Опрос.** Тот же `GET {base}/downloads` с `?fragment=1` (или `HX-Request: true`),
интервал 5 с; при скрытой вкладке опрос приостанавливается.

### 5.4. Конкурентность

| Ресурс | Защита |
|---|---|
| `Store.sessions` | `sync.RWMutex`; lookup под RLock, работа с сессией — после |
| `Session.Screens` | мьютекс сессии только на поиск/создание экрана |
| `Screen.View` | мьютекс экрана; рендер и мутации под ним |
| Сетевые вызовы | никогда не под мьютексом; применяются по `gen` |
| Снимок загрузок | отдельный мьютекс + TTL 2 с (coalescing нескольких вкладок) |
| `downloads.Updater` | остаётся только у Telegram; web его не использует |

Правило: мьютекс защищает данные, а не ожидание. Все внешние вызовы имеют контекст и
дедлайн; отмена запроса клиентом отменяет и upstream-вызов.

### 5.5. Жизненный цикл

```
MustLoadSettings
  │
  ├─ web.enabled? ──► net.Listen (ошибка = fatal, это ошибка конфига)
  │                     └─ go srv.Serve()
  ├─ bot.New (ошибка: web off → fatal; web on → log + web-only)
  │                     └─ go b.Start(ctx)
  └─ <-ctx.Done()
        ├─ srv.Shutdown(10s)
        └─ отмена ctx бота
```

Веб-сервер стартует первым: даже если Telegram-токен/сеть сломаны, UI уже доступен.
`{base}/healthz` отвечает независимо от Telegram и внешних сервисов.

### 5.6. Поддомен (основной вариант) и `base-path` (fallback)

Основной деплой — `https://torrentino.example.com/`. Это отдельный origin,
поэтому скрипты двух других приложений на `example.com` не могут читать наши
страницы и CSRF-токены (риск T15 закрыт). В этом режиме:

- `web.base-path: "/"` — префикса нет, `url(path)` возвращает путь как есть
  (`/find`, `/static/...`), `StripPrefix` не используется;
- cookie сессии — host-only, без `Domain`, `Path=/`; на базовый домен она не уходит;
- nginx — отдельный `server`-блок поддомена, `proxy_pass http://127.0.0.1:8088;`
  в `location /` (см. 08, §5.3);
- старый путь `https://example.com/torrentino/` — только `301` на поддомен.

`web.base-path` остаётся в коде как fallback/для локальной разработки и для случая,
если поддомен недоступен (`{base}` ниже — префикс; в основном деплое он пуст):

- значение нормализуется: ведущий `/`, без хвостового `/`, без `..`, `//`,
  query/fragment; допустимы `/` и, например, `/torrentino`;
- внешний handler принимает только пути под `base`: точный `base` и `base/` переводятся
  на `base/find`; `base/...` → `http.StripPrefix(base, mux)`, внутри mux маршруты
  остаются `/find`, `/ui/...`, `/static/...`; всё остальное — `404`;
- все URL генерируются через helper `url(path)`: при `base="/"` возвращается `path`
  как есть, иначе `base + path`; helper используют ссылки, формы, htmx (`hx-post`,
  `hx-push-url`), `HX-Redirect`, `Location`, статика, `next` после логина;
- cookie сессии и pre-auth cookie — `Path=base`; logout удаляет с тем же `Path`;
- `next` принимается, только если это путь под `base`; иначе — `base/find`;
- nginx передаёт путь без обрезки: `proxy_pass http://127.0.0.1:8088;` без URI.

При возврате к пути снова появляется same-origin риск (06, §15), поэтому основной
вариант — поддомен; путь не считается равноценной заменой, только совместимым fallback.

## 6. Ошибки

- `core` возвращает обёрнутые ошибки (`github.com/pkg/errors`, как принято) и несколько
  sentinel-ошибок: `ErrStaleItem`, `ErrNotAvailable`, `ErrUpstream`.
- `web` преобразует их в:
  - `409` + «list changed, reload» для устаревшего индекса/исчезнувшего элемента;
  - `400` для синтаксически неверной команды;
  - `403` для CSRF/Origin;
  - `401` + `HX-Redirect: {base}/login` для истёкшей сессии;
  - `502` для недоступного upstream (с безопасным текстом, без внутренних деталей);
  - для htmx-команд ошибка действия отдаётся фрагментом с баннером (200), чтобы
    интерфейс не «ломался».
- Внутренние детали и стек — только в лог, вместе с request-id; клиенту — короткий текст.

## 7. Производительность и память

- Рендер страницы — микросекунды; узкое место — Jackett (до 60 с) и обход диска.
- Снимок `/downloads` кэшируется на 2 с: при 5-секундном опросе и нескольких вкладках
  Transmission и диск опрашиваются не чаще ~0,5 раза в секунду.
- Обход диска вне мьютекса; при ошибке сохраняется предыдущий список и показывается
  предупреждение.
- Результаты поиска живут в сессии до нового поиска; при большом количестве (сотни
  элементов с полями `Description`) может понадобиться `web.max-results` — вопрос Q9
  в 10, в v1 жёсткого лимита нет.
- Сессии ограничены: максимум 512, TTL, GC раз в минуту; логин ограничен rate limit.

## 8. Что остаётся за рамками веб-сервера

- Telegram-хендлеры и `paginator.Paginator` продолжают работать как раньше (после Phase 0
  они используют `core` и `list.View`).
- Глобальный `downloads.Updater` не трогаем; его известное ограничение «один updater
  на процесс» остаётся проблемой только Telegram.
- Существующий дефект ACL для callback-ов (AGENTS.md п. 13) не исправляется этим
  проектом; web от него не зависит. Исправление можно сделать отдельно.
