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
Объём — ~2000 строк Go, тестов нет.

## Сборка и запуск

```bash
go build ./...          # сборка (проверено: OK)
go vet ./...            # статический анализ (проверено: чисто)
go build -v -ldflags "-s -w"   # релизная сборка (как в update-service.sh)
```

Запуск — **только из каталога, где лежит `settings.json`**: конфиг читается по
относительному пути `./settings.json` через `init()` в `common/settings.go`.

```bash
./torrentino
```

Тестов и линтер-конфигов в проекте нет. Единственный способ проверки изменений —
`go build ./...` + `go vet ./...` + ручная проверка в Telegram.

## Структура

```
main.go                     точка входа: создание бота, ACL-middleware, роутинг команд
settings.json               конфиг (в .gitignore, в репозиторий не коммитится)
common/
  settings.go               загрузка ./settings.json в глобальную common.Settings
  utils/utils.go            FormatFileSize, ReadDir (канал), WithTimeout, LogError
  ordmap/ordmap.go          generic-ordered map с iter.Seq2 (итераторы Go 1.23+)
  paginator/paginator.go    ядро UI: сообщение + inline-клавиатура, пагинация, сортировка, фильтры, действия
  paginator/list.go         List: срез индексов, Evaluator, Sorting, очередь мультисортировки
api/
  jackett/jackett.go        поиск, парсинг .torrent для InfoHash, cookiejar
  transmission/transmission.go  тонкие обёртки над transmissionrpc/v2
  torrserver/torrserver.go  JSON-over-HTTP POST /torrents
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
  (печатает файл, строку и имя функции).
- Весь внешний ввод оборачивается в `utils.WithTimeout` — таймауты 3–30 с.
- UI-текст — HTML (`ParseModeHTML`), с emoji-индикаторами: 📥 в Transmission,
  🎦 в Torrserver, 🧲 magnet, 📎 .torrent, 🎬/🎧 по расширению.
- Комментарии в коде — смесь английского и русского.

## Подводные камни

Стоит знать до правок:

1. **Путь к конфигу.** `./settings.json` — относительно CWD. Запуск из другого каталога
   даёт пустой `common.Settings` и падение/мусорные URL, а не внятную ошибку.
2. **`settings.json` в рабочем дереве содержит реальные секреты** (токен бота, список
   user id). Файл в `.gitignore` и не отслеживается git — не коммитить его и не копировать
   значения из него в код, документацию и ответы.
3. **`readme.md` устарел:** в примере конфига указан `download_dir`, а код ожидает
   объект `path` с полями `default` / `movie` / `series`.
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
   `transmissionrpc/v3`.
9. **Torrserver:** JSON собирается конкатенацией строк без экранирования — название
   раздачи с кавычками ломает запрос. В `api/torrserver/torrserver.go` есть
   неиспользуемая (и незакоммиченная) функция `httPost`, дублирующая логику `List`.
10. **Только Unix:** `syscall.Statfs` в футере `/downloads` делает сборку под Windows
    невозможной без правок.
11. **`utils.LogError`** предполагает путь пакета с префиксом (`parts[0][11:]`) — вызов
    из нестандартного окружения может дать мусорное имя.

## Деплой

`torrentino.service` ожидает бинарник в `/opt/torrentino`, логи пишутся в
`/var/log/torrentino.log`, `Restart=always`. Обновление — `sudo ./update-service.sh`
(собирает с `-ldflags "-s -w"`, останавливает сервис, подменяет бинарник, запускает).
