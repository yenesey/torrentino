# 04. HTTP-контракт

## 1. Принципы

- **Same-origin, HTML over the wire.** В v1 сервер отдаёт страницы и HTML-фрагменты,
  а не JSON. Состояние экрана хранится на сервере; URL — точка входа, а не полное
  описание состояния.
- **Всё относительно `web.base-path`.** Основной деплой — поддомен
  `https://torrentino.example.com/` с `base-path: "/"`, поэтому пути в документе
  совпадают с URL: `GET /find`. Префикс (`/torrentino`) остаётся поддерживаемым
  fallback-режимом: тогда URL получают префикс, и cookie, редиректы, статика и `next`
  используют его же. В тексте `{base}` — этот префикс без хвостового `/`; для основного
  деплоя он пуст (`{base}/find` = `/find`), для fallback — `/torrentino`.
- **Никаких мутаций через GET.** Все действия, меняющие состояние (загрузка, пауза,
  удаление, вход/выход), — только `POST` с CSRF-токеном.
- **PRG для обычных форм.** `POST` без htmx завершается `303` на `GET`-страницу;
  htmx-запрос получает фрагмент.
- **Один фрагмент — один экран.** Все команды возвращают HTML для `#screen`
  (outerHTML), поэтому разметка не зависит от того, какая кнопка нажата.
- **Ошибки не ломают UI.** Ошибки действий отдаются фрагментом с баннером; коды
  4xx/5xx — только для нарушений протокола, авторизации и недоступного upstream.

## 2. Аутентификация и сессия

| Метод | Путь | Доступ | Назначение |
|---|---|---|---|
| GET | `/login` | аноним | форма входа |
| POST | `/login` | аноним, rate limit | `username`, `password`, `csrf`, `next` |
| POST | `/logout` | сессия | инвалидация сессии, `303 /login` |

- Cookie сессии: `torrentino_session`, случайные 32 байта, `HttpOnly`, `SameSite=Lax`,
  `Path=<base>`, `Secure` при TLS/прокси, `Max-Age` = absolute TTL. Сервер хранит только
  SHA-256 токена.
- CSRF: токен сессии в `<meta name="csrf-token">`, в скрытом поле `csrf` каждой формы и
  в заголовке `X-CSRF-Token` (его добавляет `app.js` через `htmx:configRequest`).
- `next` принимается только как путь под `base` (для поддомена — например,
  `/downloads`; для fallback — `/torrentino/downloads`), без схемы, хоста, `//`, `..`;
  иначе игнорируется и используется `/find` (в fallback — `/torrentino/find`).
- Истёкшая сессия:
  - обычный запрос → `303 /login?next=<path>`;
  - htmx-запрос → `200` + `HX-Redirect: /login?next=<path>` (фрагмент пустой),
    чтобы клиент ушёл на логин, а не вставил страницу логина в `#screen`.
  В fallback-режиме к путям добавляется `/torrentino`.
- Подробности паролей/сессий/лимитов — в [06](06-auth-and-security.md).

## 3. Эндпоинты

`{kind}` ∈ `find` | `downloads` | `torrserver`. Неизвестный kind → `404`.
Все пути — относительно `web.base-path`. В основном деплое (`base-path: "/"`) это
корень поддомена; для fallback-режима добавьте `/torrentino` к каждому пути.

| Метод | Путь | Тело/параметры | Ответ |
|---|---|---|---|
| GET | `/` | — | `303 /find` |
| GET | `/find` | `fragment=1` (опц.) | страница или фрагмент `#screen` |
| POST | `/find/search` | `q`, `csrf` | htmx: фрагмент + `HX-Push-Url: /find`; иначе `303 /find` |
| GET | `/downloads` | `fragment=1` | страница/фрагмент; каждый запрос обновляет данные (кэш 2 с) |
| GET | `/torrserver` | `fragment=1` | страница/фрагмент |
| POST | `/ui/{kind}/command` | `cmd`, `csrf` | фрагмент `#screen` |
| GET | `/find/torrent` | `item=<i>` | файл `.torrent` |
| GET | `/healthz` | — | `200 {"status":"ok"}` |
| GET | `/static/*` | `?v=<hash>` | CSS/JS/htmx, долгий cache |

`fragment=1` эквивалентен заголовку `HX-Request: true`; оба способа поддержаны, чтобы
опрос работал и без htmx (например, из скрипта).

## 4. Команды экрана

`cmd` — одна строка. Индексы в протоколе **0-based**, в UI отображаются как `i+1`
(как в Telegram callback-данных).

| `cmd` | Параметры | Проверки | Действие |
|---|---|---|---|
| `select:<i>` | `i` — десятичное | не цифры/переполнение → `400`; `i >= Len` → `409` | выбрать элемент, закрыть панель управления |
| `deselect` | — | — | снять выбор |
| `page:next` / `page:prev` | — | на границе — no-op | сменить страницу |
| `controls:toggle` | — | — | показать/скрыть сортировки и фильтры |
| `sort:<attr>` | attr из `core.*Sorting()` | attr неизвестен → `400` | цикл 1→2→0, снять выбор |
| `filter:<attr>/<value>` | attr из `core.*Filters()` | attr неизвестен → `400`; value неизвестен → no-op | переключить фильтр, `Page=0`, `Selected=-1` |
| `action:<name>` | name из `AvailableActions` | нет выбора → `409`; действие недоступно → `409` | выполнить действие |

Действия по экранам:

| Экран | `action:` |
|---|---|
| find | `download`, `download:series`, `download:movie`, `torrsrv` |
| downloads | `pause`, `start`, `delete` |
| torrserver | `delete` |

`.torrent` и `web page` в поиске — не команды: это обычная ссылка на
`base/find/torrent?item=N` и внешний `Details` (открывается в новой вкладке).

### 4.1. Удаление

`action:delete` выполняется **сразу**, как в Telegram: отдельного подтверждения нет.
Сервер:

- берёт выбранный элемент по стабильному ключу (`id:` торрента, `path:` файла на диске,
  `hash:` Torrserver), а не только по индексу;
- проверяет, что элемент всё ещё есть в текущем списке; если исчез — `409`
  («list changed»), удаления нет;
- для файла на диске проверяет, что `Clean(DownloadDir/Name)` лежит внутри одного из
  `path.*`; иначе `409` и запись в лог;
- для Transmission вызывает `DeleteLocalData: true`, для Torrserver — `rem`;
- пишет аудит (кто, что, путь/ID, результат).

Защита от чужого запроса — сессия + CSRF + `Origin`; от случайного клика — только
осознанное решение владельца (паритет с Telegram). Риск зафиксирован в 06 и 10.

## 5. `GET {base}/find/torrent?item=N`

`N` — 0-based индекс в текущем отфильтрованном списке экрана `find` (в UI отображается
как `N+1`).

- Под мьютексом экрана проверяются границы и берётся копия `Link`/`Title`.
- Вне мьютекса `core.TorrentFile(ctx, item)`:
  - только `http`/`https` из результата поиска (URL не принимается от клиента);
  - дедлайн 30 с, лимит 25 МБ;
  - проверка, что тело начинается с `d` (bencode-словарь); иначе `502`;
  - body всегда закрывается.
- Ответ: `Content-Type: application/x-bittorrent`,
  `Content-Disposition: attachment; filename*=UTF-8''<sanitized>.torrent`,
  `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`.
- Имя файла очищается от CR/LF, кавычек, `\`, `/`, управляющих символов, ограничивается
  длиной (например, 150 символов); пустое имя заменяется на `torrent`.
- Ошибки: `409` — список изменился/индекс вне границ; `404` — элемента нет; `502` —
  трекер/Jackett не отдали файл; `413` — больше лимита.

## 6. Формат страниц и фрагментов

Полная страница (`GET`, без `HX-Request`):

```html
<!doctype html>
<html lang="en">
  <head>… <meta name="csrf-token" content="…"> …</head>
  <body>
    <header>nav + user + logout</header>
    <main id="screen"> … фрагмент экрана … </main>
    <script src="{{base}}/static/htmx.min.js?v=…"></script>
    <script src="{{base}}/static/app.js?v=…"></script>
  </body>
</html>
```

Фрагмент (`#screen`) содержит:

1. шапку списка (`results: a-b of N`, «the list is empty»);
2. панель управления (сортировки/фильтры) — по `View.Controls`;
3. строки элементов с кнопкой выбора/ссылками;
4. пагинацию `Prev | page X of Y | Next`;
5. панель действий выбранного элемента;
6. баннер ошибки/успеха, если есть;
7. отметку «updated HH:MM:SS» для `/downloads`.

Правила рендера:

- `html/template` с автоэкранированием; никаких `template.HTML` из данных трекера;
- ни одной inline-строки JS в шаблонах (CSP);
- кнопки — `<button>`/`<a>`, формы — `<form method="post" action="…">` с `hx-post`,
  чтобы работало и без JS;
- htmx-атрибуты: `hx-target="#screen" hx-swap="outerHTML"`,
  `hx-disabled-elt="this"` для защиты от двойного клика;
- при ошибке действия фрагмент содержит `role="alert"` с текстом и кнопкой Retry/OK.

## 7. Заголовки и коды

| Код | Когда |
|---|---|
| 200 | страница/фрагмент, в т.ч. ошибка действия в баннере |
| 303 | PRG после POST без htmx; редирект после логина/логаута (с base-path) |
| 400 | неизвестная команда, плохой attr, битый индекс (не цифры), слишком длинный `q` |
| 401 | нет/истекла сессия |
| 403 | нет CSRF или Origin не совпадает |
| 404 | неизвестный путь/kind/элемент торрент-ссылки; запрос вне base-path |
| 405 | метод не разрешён |
| 409 | устаревший индекс, элемент исчез, недоступное действие, список изменился |
| 413 | тело запроса больше лимита |
| 429 | превышен rate limit (логин/команды) |
| 500 | внутренняя ошибка (без деталей) |
| 502 | Jackett/трекер/Torrserver вернули ошибку или мусор |

Заголовки ответов (middleware):

- `Cache-Control: no-store` для всего, кроме `{base}/static/*` и `{base}/healthz`;
- `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
  `X-Frame-Options: DENY`, CSP (см. 06), HSTS при TLS;
- `X-Request-ID` (генерируется, если не пришёл; пишется в лог);
- статика: `Cache-Control: public, max-age=31536000, immutable`, версия в `?v=`.

## 8. Соответствие Telegram-командам

| Callback-данные Telegram | Web `cmd` |
|---|---|
| `<prefix><i>` | `select:<i>` |
| `<prefix>next_page` | `page:next` |
| `<prefix>prev_page` | `page:prev` |
| `<prefix>toggle_filters` | `controls:toggle` |
| `<prefix>#order_by#<attr>` | `sort:<attr>` |
| `<prefix>#filterby#<attr>/<value>` | `filter:<attr>/<value>` |
| `<prefix>#action__#<name>` | `action:<name>` |
| `<prefix>stub` | не используется |

Парсинг web-команд — отдельная функция с allowlist-ом и тестами (как
`parseItemIndex`/`dispatch` в пагинаторе): никакого индексирования без проверки длины,
никакого `Atoi` без проверки ошибки, неизвестные команды — `400`.

## 9. Лимиты и таймауты

| Что | Значение | Где |
|---|---|---|
| Тело запроса | 64 КБ | `http.MaxBytesReader` |
| `q` | 256 символов | валидация |
| Страница списка | 5–100, по умолчанию 20 | `web.page-size` |
| Поиск Jackett | 60 с (настраивается) | `jackett.timeout` |
| Torrent | 30 с, 25 МБ | `core.TorrentFile` |
| Постер | 10 с | `core` |
| Transmission/диск | 10 с | `core` |
| `ReadHeaderTimeout` | 5 с | `http.Server` |
| `ReadTimeout` | 30 с | `http.Server` |
| `WriteTimeout` | 90 с (поиск до 60 с + запас) | `http.Server` |
| `IdleTimeout` | 120 с | `http.Server` |
| `MaxHeaderBytes` | 16 КБ | `http.Server` |
| Сессия: idle / absolute | 12 ч / 168 ч (настраивается) | 06 |
| Cookie | `HttpOnly`, `SameSite=Lax`, `Path=<base>`, `Secure` при TLS | 06 |

## 10. Примеры

Ниже `base = /` — основной деплой на
`https://torrentino.example.com/`. Для fallback с префиксом замените пути на
`/torrentino/...`.

Поиск (htmx):

```http
POST /find/search HTTP/1.1
Host: torrentino.example.com
HX-Request: true
Content-Type: application/x-www-form-urlencoded

q=matrix&csrf=<token>
```

Ответ: `200`, `HX-Push-Url: /find`, тело — фрагмент `#screen` с результатами.

Выбор и действие:

```http
POST /ui/find/command HTTP/1.1
Content-Type: application/x-www-form-urlencoded

cmd=select%3A3&csrf=<token>
```

Затем `cmd=action%3Adownload%3Amovie&csrf=<token>` — фрагмент с обновлённой строкой
и отметкой `📥`.

Удаление загрузки (сразу, как в Telegram):

```http
POST /ui/downloads/command   cmd=action%3Adelete&csrf=<token>
→ 200, фрагмент списка без элемента (или 409, если элемент уже исчез)
```

Ошибка устаревшего индекса:

```http
POST /ui/downloads/command   cmd=select%3A99
→ 409, фрагмент «list changed, reload» (кнопка Reload)
```

Редирект на логин при истёкшей сессии (htmx):

```http
200 OK
HX-Redirect: /login?next=/downloads
```

## 11. Будущее JSON API

В v1 JSON-эндпоинтов нет: их добавление без реального клиента — лишняя поверхность.
Если будет выбран SPA/WASM-клиент (см. 07), рядом с HTML-слоем добавляются
`{base}/api/screens/{kind}`, `{base}/api/screens/{kind}/commands`, `{base}/api/find`,
`{base}/api/find/torrent`, использующие те же `core` и `list.View`; сессии/CSRF остаются
теми же. HTML-эндпоинты при этом можно оставить как fallback.
