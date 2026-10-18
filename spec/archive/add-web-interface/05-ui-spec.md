# 05. UI-спецификация

Цель — не скопировать Telegram-сообщения, а дать те же возможности в удобном для браузера
виде. Тексты интерфейса — английские, как в боте; строки собраны в шаблонах, чтобы позже
добавить локализацию.

## 1. Каркас

```
┌──────────────────────────────────────────────────────────────┐
│ Torrentino   Find   Downloads   Torrserver        denis  ⏻  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│   <main id="screen"> … экран … </main>                       │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ build <hash> · updated 12:34:56                              │
└──────────────────────────────────────────────────────────────┘
```

- `header`: название, навигация (текущий экран подсвечен), имя пользователя, кнопка
  `Log out` (POST-форма).
- `main#screen` — единственная область, которую заменяют htmx-ответы.
- `footer`: версия сборки (`debug.ReadBuildInfo`) и время последнего обновления данных
  (для `/downloads` и `/torrserver`).
- Все ссылки и формы строятся от `web.base-path`: в основном деплое на
  `torrentino.example.com` это `/`, в fallback-режиме — `/torrentino`;
  хардкода `/find` в шаблонах нет.
- Мобильная версия: навигация иконками/короткими подписями, строки списка в две линии,
  кнопки не меньше 44×44 px.

## 2. Экран входа `/login`

```
        ┌────────────────────────────┐
        │        Torrentino          │
        │  Username [____________]   │
        │  Password [____________]   │
        │  [ Log in ]                │
        │  <error banner>            │
        └────────────────────────────┘
```

- Ошибка входа общая: «invalid username or password» (без указания, что именно неверно).
- Rate limit: «too many attempts, try again in N s».
- После логина — `next` (если это локальный путь) или `/find`.
- Полей регистрации/восстановления нет.

## 3. Экран поиска `/find`

```
┌──────────────────────────────────────────────────────────────┐
│ Query [matrix____________________________]  [ Search ]       │
├──────────────────────────────────────────────────────────────┤
│ results: 1-20 of 137                    [▾ controls]         │
│                                                              │
│  1. The Matrix [1.2 GB] [rutor] [12s/3p] 📎 🧲 📥            │
│  2. The Matrix Reloaded [1.4 GB] [rutracker] [8s/1p] 📎      │
│ ▶3. The Matrix Revolutions [1.5 GB] [kinozal] [3s/0p] 🧲     │
│                                                              │
│         [◀ Prev]   page 1 of 7   [Next ▶]                    │
│                                                              │
│  selected #3: [download] [download:series] [download:movie]  │
│               [torrsrv] [.torrent] [web page]                │
└──────────────────────────────────────────────────────────────┘
```

- Форма: `Query` + `Search`; Enter отправляет. Во время запроса — индикатор
  `Searching… Ns`, кнопка disabled (поиск может идти до 60 с).
- Пустой запрос не отправляется; после ошибки — баннер с текстом и `Retry`.
- Строка результата: номер, `Title`, `[size]`, `[TrackerId]`, `[Ns/Mp]`, маркеры:
  `📎` есть `.torrent`, `🧲` magnet, `📥` уже в Transmission, `🎦` уже в Torrserver.
  Маркеры имеют `title`/`aria-label` («in Transmission», «magnet link», …).
- Клик по строке — выбор (как кнопка-номер в Telegram); повторный клик по выбранной
  строке снимает выбор. Выбранная строка подсвечена, её номер повторён в панели действий.
- Панель действий показывается только для выбранной строки; состав кнопок — из
  `core.AvailableActions` (см. 02, F6).
- `download`, `download:series`, `download:movie` — POST-команды; после успеха на строке
  появляется `📥`, выбор снимается, показывается короткий success-баннер.
- `torrsrv` — POST; может занять до 10 с (скрейпинг постера); на время запроса кнопка
  disabled и показывает `Adding…`; ошибка скрейпинга не мешает добавлению.
- `.torrent` — обычная ссылка на `/find/torrent?item=N` (скачивание браузером).
- `web page` — внешняя ссылка `Details` (`target="_blank"`, `rel="noopener noreferrer"`).
- Если `InfoHash` ещё не известен, при выборе строка на мгновение показывает
  `Resolving…`; ошибка отображается мелким текстом в баннере, действия остаются.

### 3.1. Панель управления

- Кнопка `controls` раскрывает блок «Sort» / «Filter».
- Sort: кнопки `size ▼`, `seeds ▼`, `peers ▼`, `file ▼`; клик циклит
  `▼ (desc) → ▲ (asc) → off`. Активная сортировка показывает стрелку; при мультисортировке
  рядом со стрелкой — номер в очереди (`▼1`, `▲2`).
- Filter: по трекеру, кнопки-чипы `✓ rutor`, `rutracker`, …; клик переключает; при
  включённых чипах показываются только они. Кнопка `Clear filters`.
- Смена сортировки/фильтра сбрасывает выбор; фильтр также сбрасывает страницу.
- Пустой результат: «No results for “matrix”» + кнопка `Clear filters`.

## 4. Экран загрузок `/downloads`

```
┌──────────────────────────────────────────────────────────────┐
│ Downloads                             updated 12:34:56  ⟳    │
├──────────────────────────────────────────────────────────────┤
│ results: 1-20 of 42                    [▾ controls]          │
│                                                              │
│  1. 🎬 Some.Movie.2024.1080p [12.4 GB] [100%] [2.13x]        │
│        [seeding:4p]                                   📥     │
│  2. 🎧 Album [640 MB] [37%] [0.12x] [downloading:8p]         │
│  3. 📁[12x .mkv] Old.Series [48 GB] [on disk]                │
│                                                              │
│         [◀ Prev]   page 1 of 3   [Next ▶]                    │
│                                                              │
│  selected #2: [pause] [delete]                               │
│                                                              │
│  ⸻ 1.2 TB downloaded / 480 GB uploaded · volume: 8.1 TB     │
│     used / 1.9 TB free                                       │
└──────────────────────────────────────────────────────────────┘
```

- Строка: иконка расширения, имя, `[size]`, `[percent%]`, `[ratio x]`, бейдж статуса
  (`seeding:4p`, `downloading:8p`, `paused`, `idle`, `on disk`).
- `📁[Nx .ext]` — директория; `on disk` — элемент, найденный на диске и отсутствующий
  в Transmission (внутренний статус `unknown`).
- Действия: `pause` для seeding/downloading, `start` для остальных известных,
  `delete` всегда. Для `on disk` — только `delete` (удаление файлов!).
- `delete` удаляет сразу, как в Telegram. Кнопка красная, с подписью `delete` и
  `title="Delete now — cannot be undone"`; отдельного диалога нет.
- Автообновление: htmx-запрос фрагмента каждые 5 с. Индикатор `⟳` показывает, что
  обновление включено; при скрытой вкладке опрос приостанавливается и индикатор гаснет.
- Если данные не обновились — старая таблица остаётся, в шапке появляется
  «update failed, retry in Ns»; кнопка `Retry now`.
- Футер: `downloaded / uploaded` по списку и `volume: used / free` по `path.default`.
  При ошибке `Statfs` — только downloaded/uploaded, без объёма.

## 5. Экран Torrserver `/torrserver`

```
┌──────────────────────────────────────────────────────────────┐
│ Torrserver                              updated 12:34:56     │
│ results: 1-20 of 7                    [▾ controls]           │
│                                                              │
│  1. Some.Movie.2024.1080p [12.4 GB]                          │
│  2. Old.Series.S01 [48 GB]                                   │
│                                                              │
│         [◀ Prev]   page 1 of 1   [Next ▶]                    │
│  selected #1: [delete]                                       │
└──────────────────────────────────────────────────────────────┘
```

- Только сортировка `size` (по умолчанию desc); фильтров нет.
- `delete` удаляет запись Torrserver сразу, как в Telegram; файлы на диске не трогаются
  (это указано в `title` кнопки).
- Автообновления нет (как в Telegram); кнопка `Refresh` в шапке экрана.

## 6. Удаление

Отдельного подтверждения нет — паритет с Telegram:

```
selected #2: [pause] [delete]
```

- `delete` — обычная POST-команда; при успехе элемент исчезает из списка, показывается
  success-баннер «Deleted “…”».
- Для элемента только на диске удаляются файлы (`os.Remove`/`os.RemoveAll`); в строке
  видны имя и размер, кнопка имеет `title="Delete files now — cannot be undone"`.
- Для Transmission удаляются торрент и локальные данные; для Torrserver — только запись.
- Ошибка — баннер с текстом, элемент остаётся в списке.
- Защита: сессия, CSRF, проверка пути на сервере, аудит. Случайный клик возможен —
  риск принят владельцем (см. 06, §15); в будущем возможна read-only роль или корзина.

## 7. Состояния

| Состояние | Вид |
|---|---|
| Загрузка страницы | скелетон/`Loading…` (быстро, обычно не виден) |
| Поиск | `Searching… Ns`, форма disabled, отмена при уходе со страницы |
| Пустой список | «the list is empty» (для поиска — «No results for …») |
| Ошибка загрузки списка | баннер `role="alert"`, текст, `Retry` |
| Ошибка действия | баннер над списком, список не теряется |
| Устаревший индекс (409) | баннер «list changed», кнопка `Reload` |
| Истёкшая сессия | редирект на `base/login?next=…` |
| Нет JS | формы работают обычным POST + 303; автообновления нет, есть `Refresh` |

## 8. Доступность и клавиатура

- Семантика: `<header>`, `<nav>`, `<main>`, `<form>`, `<button>`, `<ul>`/`<table>` для
  списка; заголовки `h1`/`h2`.
- Все действия доступны с клавиатуры; фокус после htmx-замены переносится на заголовок
  списка или на баннер ошибки (`autofocus`/JS).
- `aria-live="polite"` для времени обновления и success-баннеров, `role="alert"` для ошибок.
- Emoji не единственный носитель смысла: у маркеров есть текстовые `aria-label`/`title`.
- Контраст текста ≥ 4.5:1; размер шрифта не меньше 14 px; кнопки-иконки имеют подписи.
- Уважать `prefers-reduced-motion` (не анимировать обновления).

## 9. Визуальный стиль

- Один CSS-файл, без внешних шрифтов и CDN; системный шрифт.
- Светлая/тёмная тема через `prefers-color-scheme`, без переключателя в v1.
- Палитра: нейтральный фон, акцент для выбранной строки и основных кнопок, красный —
  только для destructive.
- Таблицы/строки не «прыгают» при обновлении: фиксированные колонки размера/статуса.

## 10. Тексты (v1)

| Ключ | Текст |
|---|---|
| nav.find / downloads / torrserver | Find / Downloads / Torrserver |
| list.results | `results: %d-%d of %d` |
| list.empty | `the list is empty` |
| search.placeholder | `Search torrents…` |
| search.button | `Search` |
| search.searching | `Searching… %ds` |
| search.noResults | `No results for %q` |
| actions.download / series / movie | `download` / `download:series` / `download:movie` |
| actions.torrsrv | `torrsrv` |
| actions.torrent | `.torrent` |
| actions.web | `web page` |
| actions.pause / start / delete | `pause` / `start` / `delete` |
| actions.delete.title | `Delete now — cannot be undone` |
| error.stale | `The list changed. Reload and try again.` |
| error.updateFailed | `Update failed. Showing the previous data.` |
| login.invalid | `Invalid username or password` |
| login.rateLimited | `Too many attempts. Try again in %d s.` |

Подписи действий намеренно совпадают с Telegram (включая `torrsrv`) — это упрощает
сверку паритета; косметическое переименование возможно отдельным решением.

## 11. Ассеты

- `web/static/app.css` — стили.
- `web/static/app.js` — ~50–80 строк: CSRF-заголовок для htmx, visibility-aware опрос
  `{base}/downloads`, перенос фокуса после swap, защита от двойной отправки.
- `web/static/htmx.min.js` — вендорный файл фиксированной версии (2.x), лицензия BSD;
  обновление — осознанное действие с проверкой changelog. `htmx.config.allowEval = false`,
  поэтому inline-выражения/`hx-on` не используются.
- Все файлы встроены через `embed` и отдаются с версией в query-параметре.
