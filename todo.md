# RefLink — план работы (MVP)

> Статус: **MVP реализовано** (2026-11-01). Сервер для ручной проверки UI:
> `go run ./cmd/reflink` → http://localhost:8080.
> Этот файл — рабочий чек-лист: отмечаем выполненное по ходу работы.

## 1. Суть продукта

Веб-приложение: пользователь загружает фотографию и сразу видит её разбор —
общие данные (формат, разрешение, размер файла), параметры камеры и съёмки
из EXIF и геоданные со ссылкой на карту, если они есть в файле.

Изображения **не сохраняются** — обработка в памяти, файл нигде не остаётся.
Анализ изображения (цветокоррекция, палитра и т.п.) в MVP **не входит** —
только загрузка и общие данные.

## 2. Зафиксированные решения

| Вопрос | Решение |
|---|---|
| Форматы | JPEG, PNG, WebP, GIF, BMP |
| EXIF | JPEG — полный парсер; PNG (chunk `eXIf`) и WebP (chunk `EXIF`) — парсим, если присутствует; в GIF/BMP EXIF не бывает |
| Геоданные | Координаты (десятичные + DMS) и ссылка на OpenStreetMap |
| Язык интерфейса | Английский |
| Файлов за раз | Один |
| Лимит размера | 20 МБ |
| Логи | Только файл `uploads.log` |
| Деплой | Ubuntu 24.04, systemd-сервис, Caddy (HTTPS + reverse proxy) |
| Структура | `cmd/` + `internal/` |
| Тема | Только тёмная |
| Хранение / БД / авторизация | Нет, не в MVP |

## 3. Вне рамок MVP (осознанно)

- Анализ цветокоррекции / палитры / «какие цвета исключены»
- Сохранение изображений, БД, авторизация
- Светлая тема, мультизагрузка, локализация
- HTTPS при локальной отладке (Caddy — только прод)

## 4. Стек и принципы

- **Backend:** Go, только стандартная библиотека (`net/http`, `image/jpeg`,
  `image/png`, `image/gif`). Внешних зависимостей — ноль.
- **EXIF и размеры:** собственный парсер — сегмент APP1 → TIFF → IFD0/ExifIFD/GPS IFD;
  RIFF-заголовки WebP; BITMAPINFOHEADER для BMP.
- **Frontend:** чистые HTML + CSS + JS, без фреймворков, сборщиков и CDN.
- **Простота:** минимум кода, понятные имена, никакой магии.

## 5. Структура проекта

```
reflink/
├── go.mod
├── cmd/
│   └── reflink/
│       └── main.go              # точка входа: конфиг из env, запуск сервера
├── internal/
│   ├── appcfg/
│   │   └── appcfg.go            # чтение env-конфига
│   ├── server/
│   │   ├── server.go            # маршруты, middleware, раздача статики
│   │   └── analyze.go           # POST /api/analyze (multipart, валидация, JSON)
│   ├── imageinfo/
│   │   ├── imageinfo.go         # общая модель результата
│   │   ├── detect.go            # определение формата по магическим байтам
│   │   ├── jpeg.go              # размеры (stdlib) + APP1 → EXIF-байты
│   │   ├── png.go               # IHDR (размеры) + chunk eXIf
│   │   ├── webp.go              # VP8/VP8L/VP8X (размеры) + chunk EXIF
│   │   ├── gif.go               # логический экран GIF (размеры)
│   │   └── bmp.go               # BITMAPINFOHEADER (размеры)
│   ├── exif/
│   │   ├── exif.go              # парсер TIFF/IFD (общий для JPEG/PNG/WebP)
│   │   ├── tags.go              # номера тегов + названия
│   │   └── gps.go               # GPS rational → десятичные градусы, DMS
│   └── uploadlog/
│       └── uploadlog.go         # лог загрузок в файл (мьютекс)
├── static/
│   ├── index.html
│   ├── css/style.css            # тёмная тема
│   └── js/app.js                # fetch, drag&drop, превью, карточки
├── testdata/                    # маленькие тестовые изображения
├── .gitignore                   # бинарники, uploads.log
├── README.md                    # описание, запуск, API, конфиг
├── DEPLOY.md                    # инструкция по развертыванию на VDS
└── todo.md
```

## 6. Конфигурация (переменные окружения)

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `PORT` | `8080` | Порт HTTP-сервера |
| `MAX_UPLOAD_MB` | `20` | Лимит размера файла, МБ |
| `LOG_FILE` | `uploads.log` | Путь к файлу лога загрузок |
| `TRUST_PROXY` | `false` | `true` за Caddy: брать IP из `X-Forwarded-For` |

## 7. API

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/` | index.html |
| GET | `/static/*` | css/js |
| GET | `/api/health` | `{"status":"ok"}` — для мониторинга |
| POST | `/api/analyze` | разбор изображения (multipart, поле `file`) |

**Ответ `/api/analyze` (упрощённо):**

```json
{
  "file":  { "name": "IMG_1234.jpg", "size_bytes": 3821945,
             "size_human": "3.6 MB", "sha256": "…" },
  "image": { "format": "JPEG", "width": 4032, "height": 3024,
             "megapixels": 12.2, "aspect_ratio": "4:3",
             "orientation": "Rotate 90 CW" },
  "exif":  { "present": true,
             "make": "Apple", "model": "iPhone 15 Pro", "lens": "…",
             "date_taken": "2026-07-14 18:32:01",
             "settings": { "iso": 125, "exposure": "1/120 s",
                           "aperture": "f/1.78", "focal_length": "6.86 mm",
                           "flash": "Did not fire", "software": "…" } },
  "gps":   { "present": true, "latitude": 55.7522, "longitude": 37.6156,
             "altitude_m": 151, "dms": "55°45′8″N, 37°36′56″E",
             "map_url": "https://www.openstreetmap.org/?mlat=…&mlon=…#map=15/…" }
}
```

Если данных нет — `"present": false` в соответствующем блоке.

**Ошибки:** `400` — файл не передан / битый multipart; `413` — превышен лимит;
`415` — неподдерживаемый формат; `500` — внутренняя ошибка.

## 8. Экран (одна страница, тёмная тема, английский)

- Зона загрузки: drag&drop + кнопка «Choose file», подсказка про форматы и 20 МБ
- Превью изображения (+ учёт EXIF-ориентации через CSS transform)
- Состояние «Analyzing…», затем карточки:
  - **File** — имя, размер, SHA-256
  - **Image** — формат, W×H, мегапиксели, соотношение сторон, ориентация
  - **Camera** — производитель, модель, объектив, дата съёмки
  - **Settings** — ISO, выдержка, диафрагма, фокусное расстояние, вспышка, ПО
  - **Location** — координаты (десятичные + DMS), высота, ссылка «Open on OpenStreetMap»
- Если EXIF/GPS отсутствуют — пометка «Not found in file»
- Ошибки — понятные сообщения на странице (без `alert()`)
- Клиентская валидация типа и размера до отправки; серверная — обязательна

## 9. Безопасность и аккуратность

- Изображение читаем в память (`http.MaxBytesReader` + multipart reader),
  на диск не пишем вообще
- Имя файла не используется на диске; в UI выводим через `textContent` (анти-XSS)
- Лимит тела запроса: 20 МБ + небольшой запас на multipart-обёртку
- В лог — только факт наличия GPS, сами координаты не пишем (приватность)

## 10. Этапы работ

### Этап 1. Каркас проекта
- [x] `go mod init reflink` (Go 1.25.5, stdlib без зависимостей)
- [x] `internal/appcfg`: чтение env (PORT, MAX_UPLOAD_MB, LOG_FILE, TRUST_PROXY, STATIC_DIR)
- [x] `internal/server`: маршруты, раздача `static/`, `/api/health` (отдаёт лимит и список форматов)
- [x] `cmd/reflink/main.go`: конфиг, запуск, graceful shutdown
- [x] `.gitignore`

### Этап 2. Формат и размеры изображений
- [x] `detect.go`: магические байты JPEG / PNG / GIF / BMP / WebP
- [x] размеры: JPEG/PNG/GIF через `DecodeConfig`;
      WebP — свой парсер (VP8/VP8L/VP8X); BMP — свой парсер (DIB-заголовок)
- [x] мегапиксели, соотношение сторон (сокращение дроби через НОД)

### Этап 3. EXIF-парсер (чистый Go, без библиотек)
- [x] извлечение EXIF-байтов: JPEG APP1 (`Exif\0\0`), PNG chunk `eXIf`, WebP chunk `EXIF`
- [x] парсер TIFF: byte order (II/MM), IFD0 → ExifIFD (0x8769) → GPS IFD (0x8825)
- [x] типы данных: BYTE / ASCII / SHORT / LONG / RATIONAL / SRATIONAL
- [x] камера: Make, Model, LensMake, LensModel, Software, DateTime(Original)
- [x] съёмка: ISO, ExposureTime, FNumber, FocalLength, Flash, MeteringMode,
      WhiteBalance, Orientation, FocalLength35
- [x] GPS: Lat/Lon (DMS rational) → десятичные градусы, знак по N/S/E/W, Altitude
- [x] сервер отдаёт и «сырые» числа, и готовые строки («1/120 s», «f/1.78»)

### Этап 4. API `/api/analyze`
- [x] multipart: поле `file`, чтение в память (без временных файлов)
- [x] валидация: формат из белого списка, размер ≤ лимита
- [x] сборка JSON-ответа, коды 400 / 413 / 415

### Этап 5. Логирование загрузок
- [x] `internal/uploadlog`, строка вида:
      `2026-11-01T12:00:00Z ip=1.2.3.4 file="IMG.jpg" size=3821945 fmt=JPEG 4032x3024 exif=yes gps=no status=ok`
- [x] мьютекс, append в файл, путь из `LOG_FILE`
- [x] `TRUST_PROXY=1` → IP из `X-Forwarded-For` (первый адрес)

### Этап 6. Frontend
- [x] `index.html` + `style.css`: тёмная тема, аккуратная типографика, адаптив
- [x] drag&drop + выбор файла, клиентская валидация (тип / размер)
- [x] превью через `URL.createObjectURL` + `image-orientation: from-image`
      (EXIF-поворот применяется браузером), освобождение URL
- [x] `fetch` → POST `/api/analyze`; состояния idle / uploading / done / error
- [x] карточки File / Image / Camera / Settings / Location + копирование SHA-256
- [x] ссылка OSM в новой вкладке (`rel="noopener"`)

### Этап 7. Тесты
- [x] exif: синтетический TIFF с IFD0 + ExifIFD + GPS (байты собирает `internal/testtif`)
- [x] gps: конвертация DMS → десятичные, знак по N/S/E/W, трэп «0/0»
- [x] detect: сигнатуры всех 5 форматов
- [x] размеры: WebP/BMP на самодельных заголовках; JPEG/PNG/GIF на сгенерированных
- [x] handler: httptest + multipart — успех, пустой запрос, превышение лимита,
      неверный формат, битый JPEG, из заголовка X-Forwarded-For
- [ ] ручная проверка в браузере на реальных файлах (выполнится на Этапе 9)

### Этап 8. Документация
- [x] `README.md`: что это, запуск локально, конфиг, API + пример curl, приватность
- [x] `DEPLOY.md`: полная инструкция для VDS (см. §11)

### Этап 9. Финальная проверка
- [x] `go vet`, `gofmt -l`, `go test ./...` — чисто
- [x] сценарии через API (curl): JPEG с EXIF+GPS, JPEG без EXIF, PNG, WebP
   (VP8/VP8L/VP8X), GIF, BMP, файл > 20 МБ (413), файл-текст (415)
- [x] текущий сервер поднят, заготовки в `testdata/` — загрузка любого
   файла в UI повторит перечисленные проверки нажатием
- [ ] ручная проверка UI в браузере (десктоп-браузер не был подключён к
   сессии агента — открытая задача пользователя)
- [x] отметить галочки в todo.md, финальное ревью кода

## 11. Деплой на VDS — схема (детали в DEPLOY.md)

1. **Сборка (локально):**
   `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o reflink ./cmd/reflink`
2. **На сервере:** `/opt/reflink/` — бинарник + `static/`; системный пользователь
   `reflink` (без shell); логи — `/var/log/reflink/uploads.log`
3. **systemd:** юнит `reflink.service` (Restart=always, TRUST_PROXY=1, PORT=8080), автозапуск
4. **logrotate:** ротация `uploads.log` (weekly, compress)
5. **Caddy:** установка из официального apt-репозитория;
   Caddyfile: `reflink.example.com { reverse_proxy 127.0.0.1:8080 }` →
   автоматический HTTPS от Let's Encrypt
6. **ufw:** разрешить порты 22, 80, 443
7. **Проверка:** `systemctl status reflink`, `curl https://домен/api/health`,
   хвост `uploads.log`
8. **Обновление версии:** пересобрать → скопировать → `systemctl restart reflink`

## 12. Риски и нюансы

- EXIF в PNG/WebP встречается редко — парсим, если есть; иначе «Not found in file»
- HEIC (iPhone по умолчанию) не поддержан в MVP — контейнер сложный, добавим позже
- Без `TRUST_PROXY=1` за Caddy в логи попадёт 127.0.0.1 — включить на проде
- Битые/усечённые EXIF — парсер падает мягко: без паники, `present:false` или частичные данные

## 13. После MVP (не сейчас)

- Анализ цветокоррекции и палитры (исключённые цвета и т.д.)
- HEIC/RAW, мультизагрузка, история запросов (БД), авторизация
