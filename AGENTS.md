# AGENTS.md

Краткое описание проекта для AI-агентов и разработчиков.

## Что это

`torrentino` — Telegram-бот (Go) для управления «домашней» экосистемой загрузок на NAS.
Даёт поиск раздач, постановку загрузок, просмотр/управление списком загрузок и мгновенный
просмотр видео через Torrserver.

Внешние интеграции:
- **Jackett** — метапоисковик торрентов (API v2.0), используется как поисковый движок.
- **Transmission** — торрент-клиент: добавление, пауза/старт, удаление, список.
- **Torrserver** — стриминг видео без полной загрузки (add/list/rem).

Репозиторий: `github.com/yenesey/torrentino`. Модуль: `torrentino`. Go 1.24 (`toolchain go1.24.0`).
Объём — ~2000 строк Go. Тесты точечные: `common/settings_test.go` и
`api/torrserver/torrserver_test.go`.

## Сборка и запуск

```bash
go build ./...          # сборка (проверено: OK)
go vet ./...            # статический анализ (проверено: чисто)
go test -race ./...     # тесты (проходят)
go build -v -ldflags "-s -w"   # релизная сборка (как в update-service.sh)
```

Запуск — **только из каталога, где лежит `settings.json`**: конфиг читается по
относительному пути `./settings.json`, путь не переопределяется ни флагом, ни переменной
окружения. Загрузка выполняется явным вызовом `common.MustLoadSettings()` в начале
`main()`, а не из `init()` — см. п. 1 подводных камней.

```bash
./torrentino
```

Если конфига нет или он невалиден, бот не стартует: печатает диагностику и выходит
с кодом 1 (см. п. 1 подводных камней).

Линтер-конфигов в проекте нет. Тестов немного, они покрывают загрузку/валидацию конфига
и сборку запросов к Torrserver; остальное проверяется вручную в Telegram.

## Структура

```
main.go                     точка входа: создание бота, ACL-middleware, роутинг команд
settings.json               конфиг (в .gitignore, в репозиторий не коммитится)
common/
  settings.go               LoadSettings/MustLoadSettings: чтение, валидация, common.Settings
  settings_test.go          тесты загрузки и валидации конфига (через LoadSettingsFrom)
  utils/utils.go            FormatFileSize, ReadDir (канал), WithTimeout, LogError
  ordmap/ordmap.go          generic-ordered map с iter.Seq2 (итераторы Go 1.23+)
  paginator/paginator.go    ядро UI: сообщение + inline-клавиатура, пагинация, сортировка, фильтры, действия
  paginator/list.go         List: срез индексов, Evaluator, Sorting, очередь мультисортировки
api/
  jackett/jackett.go        поиск, парсинг .torrent для InfoHash, cookiejar
  transmission/transmission.go  тонкие обёртки над transmissionrpc/v2 (клиент через sync.Once)
  torrserver/torrserver.go  JSON-over-HTTP POST /torrents, помощник post()
  torrserver/torrserver_test.go тесты экранирования, инъекции и разбора ответа
handlers/
  search/search.go          поиск: поиск + действия download/torrsrv/.torrent/web page
  downloads/downloads.go    список загрузок + файлы на диске, автообновление раз в 5 с
  torrserver/torrserver.go  список раздач Torrserver, действие delete
torrentino.service          systemd-юнит (WorkingDirectory=/opt/torrentino)
update-service.sh           сборка → stop → mv в /opt/torrentino → start (нужен root)
```

## Архитектура

**Паттерн экрана.** Каждый экран — структура, встраивающая `paginator.Paginator`, и
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

**Доступ.** Middleware в `main.go` пропускает только `From.ID` из `users-list`; чужие
сообщения просто логируются. Первый запуск с пустым `users-list` — способ узнать свой ID
из логов.

## Соглашения

- Отступы — табы (`.editorconfig`, `indent_size = 4`).
- Один экран — один пакет в `handlers/`, внешний сервис — один пакет в `api/`.
- Общие вещи — только в `common/`.
- Ошибки оборачиваются `github.com/pkg/errors` и логируются через `utils.LogError`
  (печатает файл, строку и имя функции). Исключение — `common/settings.go`: там
  используется stdlib `errors` / `fmt.Errorf` с `%w`, а конфигурационные ошибки
  фатальны и намеренно не проходят через `utils.LogError`.
- Весь внешний ввод оборачивается в `utils.WithTimeout` — таймауты 3–30 с. Для HTTP в
  `api/torrserver` вместо этого используется контекст с дедлайном
  (`http.NewRequestWithContext`): `utils.WithTimeout` перестаёт ждать, но не отменяет
  сам запрос, оставляя висеть горутину и соединение.
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
4. **Глобальное состояние.** `common.Settings`, реестр `paginator.Handlers` и
   `downloads.Updater` — пакетные/глобальные. Один живой экран на префикс **на процесс**,
   поэтому одновременные пользователи в разных чатах перетирают друг другу одно и то же
   сообщение. `updater` в `downloads` тоже один: новый `/downloads` отменяет предыдущий.
5. **Автообновление `/downloads`** — горутина с тикером 5 с, перерисовывает сообщение
   каждые 5 секунд, пока не отменён контекст.
6. **Удаление файлов.** В `handlers/downloads` для позиций со статусом `unknown`
   (файлы на диске, которых нет в Transmission) действие `delete` вызывает
   `os.Remove`/`os.RemoveAll` напрямую — это необратимо.
7. **Хрупкий разбор callback.** `callbackHandler` обращается к `cmd[0]` без проверки длины
   (паника на пустых данных) и сравнивает байт с `unicode.IsNumber(rune(cmd[0]))` —
   работает только для ASCII-цифр.
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
10. **Только Unix:** `syscall.Statfs` в футере `/downloads` делает сборку под Windows
    невозможной без правок.
11. **`utils.LogError`** предполагает путь пакета с префиксом (`parts[0][11:]`) — вызов
    из нестандартного окружения может дать мусорное имя.
12. **Мёртвый код в `api/jackett`:** `GetValidIndexers` никто не вызывает, а его запрос
    был относительным URL (`indexers?Configured=true`) и всё равно не сработал бы;
    сейчас URL достроен до абсолютного. Мораль та же, что с `httPost`: неиспользуемые
    функции `go vet` не помечает, поэтому их надо искать глазами или через staticcheck
    (U1000).

## Деплой

`torrentino.service` ожидает бинарник в `/opt/torrentino`, логи пишутся в
`/var/log/torrentino.log`, `Restart=always`. Обновление — `sudo ./update-service.sh`
(собирает с `-ldflags "-s -w"`, останавливает сервис, подменяет бинарник, запускает).
