# RefLink — image inspector

Веб-приложение: загрузи фотографию — получи разбор. Формат и разрешение,
размер файла, данные камеры и параметры съёмки из EXIF, географические
координаты со ссылкой на карту, если они есть в файле.

**Изображения не сохраняются**: файл читается в память, разбирается и
выбрасывается. Это MVP — анализ цветокоррекции/палитры не делается, БД и
авторизация не подключаются.

Стек: **Go** — только стандартная библиотека, ноль внешних зависимостей
(включая собственный EXIF-парсер); frontend — чистые HTML/CSS/JS без
фреймворков; тёмная тема; HTTPS на проде — через Caddy.

## Возможности

- форматы: **JPEG, PNG, WebP, GIF, BMP** — формат, разрешение,
  мегапиксели, соотношение сторон;
- EXIF — полностью из JPEG, из PNG/WebP — если контейнер его содержит:
  камера (производитель, модель, объектив), ISO, выдержка, диафрагма,
  фокусное расстояние (+ 35 мм эквивалент), вспышка, замер, баланс белого,
  дата съёмки, ПО, ориентация;
- GPS: широта/долгота (десятичные градусы + ГМС), высота, ссылка
  «Open on OpenStreetMap»;
- превью с учётом EXIF-поворота, копирование SHA-256;
- лимит 20 МБ, один файл за раз, английский интерфейс;
- каждое событие загрузки пишется в лог.

## Запуск локально

Нужен Go 1.22+:

```bash
go run ./cmd/reflink
```

Открыть <http://localhost:8080>.

Сборка: `go build -o reflink.exe ./cmd/reflink` (Windows) или
`go build -o reflink ./cmd/reflink` (Linux/macOS).

Тесты: `go test ./...`

## Конфигурация (переменные окружения)

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `PORT` | `8080` | порт HTTP-сервера |
| `MAX_UPLOAD_MB` | `20` | максимальный размер загружаемого файла, МБ |
| `LOG_FILE` | `uploads.log` | путь к файлу лога загрузок |
| `STATIC_DIR` | `static` | каталог со статикой (index.html рядом с css/js) |
| `TRUST_PROXY` | `false` | берёт клиентский IP из `X-Forwarded-For`; включать **только** за доверенным прокси (Caddy) |

## API

| Метод | Путь | Описание |
|---|---|---|
| GET | `/` | одностраничный интерфейс |
| GET | `/static/*` | css/js |
| GET | `/api/health` | статус + лимиты: `{"status":"ok","max_file_mb":20,"formats":["JPEG","PNG","WebP","GIF","BMP"]}` |
| POST | `/api/analyze` | multipart/form-data, поле `file` — JSON с разбором |

Пример:

```bash
curl -F "file=@photo.jpg" http://localhost:8080/api/analyze
```

Ответ (сокращённо):

```json
{
  "file":  { "name": "IMG_1234.jpg", "size_bytes": 1128, "size_human": "1.1 KB", "sha256": "…" },
  "image": { "format": "JPEG", "width": 4, "height": 3, "megapixels": 0.01,
             "aspect_ratio": "4:3", "orientation": "Rotated 90° CW" },
  "exif":  { "present": true, "make": "Apple", "model": "iPhone 15 Pro",
             "lens": "…", "date_taken": "2026-07-14 18:32:01",
             "settings": { "iso": 125, "exposure": "1/120 s", "aperture": "f/1.78" } },
  "gps":   { "present": true, "latitude": 55.752306, "longitude": 37.615556,
             "altitude_m": 151.5, "dms": "55°45′8.3″N, 37°36′56.0″E",
             "map_url": "https://www.openstreetmap.org/?mlat=…&mlon=…" }
}
```

Если данных нет — соответствующий блок имеет `"present": false`.

Коды ошибок: `400` — не multipart / битый файл; `413` — больше лимита;
`415` — неизвестный формат; `405` — не POST; `500` — внутренняя.

## Логи

Одна строка на каждый запрос к `/api/analyze`:

```
2026-11-01T20:38:26Z ip=203.0.113.7 file="IMG_1234.jpg" size=1128 fmt=JPEG 4x3 exif=yes gps=yes status=ok
2026-11-01T20:38:36Z ip=198.51.100.2 file="notes.txt" size=19 exif=no gps=no status=denied reason=unsupported-format
```

Сами координаты в лог не пишутся (только факт их наличия).

## Структура проекта

```
cmd/reflink/         точка входа
internal/appcfg/     конфигурация из env
internal/server/     HTTP: маршруты, статика, POST /api/analyze
internal/imageinfo/  sniffing формата + размеры (JPEG/PNG/GIF stdlib; WebP/BMP — свой код)
internal/exif/       EXIF-парсер: TIFF/IFD, теги камеры и съёмки, GPS
internal/uploadlog/  лог загрузок в файл
static/              index.html, css, js (чистые, без сборки)
internal/testtif/    генератор TIFF/изображений для тестов
testdata/            примеры файлов для ручной проверки
```

## Приватность и безопасность

- изображения обрабатываются в памяти, на диск не пишутся вообще (multipart
  читается потоково, без временных файлов);
- имена файлов никогда не попадают на диск; в UI выводятся через
  `textContent`, без `innerHTML`;
- CSP без inline-скриптов/стилей, `nosniff`, `X-Frame-Options: DENY`;
- лимит тела запроса жёсткий (`http.MaxBytesReader`);
- `TRUST_PROXY` по умолчанию выключен — IP считается только из
  соединения, что не даёт подделать лог.

## Ограничения MVP

- HEIC (дефолтный формат iPhone) не поддерживается — включите в камере
  «Наиболее совместимые» или конвертируйте;
- RAW (CR2/NEF/ARW), TIFF, AVIF — вне MVP;
- цветовой анализ — следующий этап (см. todo.md).
