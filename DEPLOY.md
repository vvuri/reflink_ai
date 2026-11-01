# Развертывание RefLink на VDS (Ubuntu 24.04)

Исходные предпосылки:

- чистый VDS с Ubuntu 24.04 и sudo-пользователем (в примерах — `user`);
- домен `reflink.example.com` уже указывает A/AAAA-записью на IP сервера
  (замените домен и пользователя на свои значения);
- сборка делается **кросс-компиляцией** на машине разработки — Go на сервере не нужен;
- HTTPS выдаёт Caddy автоматически (Let's Encrypt), порты 80/443 открыты.

Итоговая схема: Caddy (443, TLS, gzip) → `127.0.0.1:8080` reflink (systemd).

## 1. Сборка

PowerShell (Windows):

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags="-s -w" -o build/reflink-linux ./cmd/reflink
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

bash (Linux/macOS):

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
go build -trimpath -ldflags="-s -w" -o build/reflink-linux ./cmd/reflink
```

Проверка: `file build/reflink-linux` → «ELF 64-bit LSB … statically linked».

## 2. Первый заход на сервер: пользователь и каталоги

```bash
ssh user@reflink.example.com

sudo useradd --system --home-dir /opt/reflink --shell /usr/sbin/nologin reflink
sudo mkdir -p /opt/reflink/static /var/log/reflink
sudo chown -R reflink:reflink /opt/reflink /var/log/reflink
```

## 3. Копирование файлов

С машины разработки:

```bash
scp build/reflink-linux user@reflink.example.com:/tmp/reflink
scp -r static user@reflink.example.com:/tmp/reflink-static
```

На сервере:

```bash
sudo mv /tmp/reflink /opt/reflink/reflink
sudo cp -r /tmp/reflink-static/. /opt/reflink/static/
sudo chown -R reflink:reflink /opt/reflink
sudo chmod 755 /opt/reflink/reflink
rm -rf /tmp/reflink-static
```

## 4. systemd-юнит

`/etc/systemd/system/reflink.service`:

```ini
[Unit]
Description=RefLink image inspector
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=reflink
Group=reflink
WorkingDirectory=/opt/reflink
ExecStart=/opt/reflink/reflink
Environment=PORT=8080
Environment=LOG_FILE=/var/log/reflink/uploads.log
Environment=STATIC_DIR=/opt/reflink/static
Environment=TRUST_PROXY=1
Restart=always
RestartSec=3

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/log/reflink
RestrictAddressFamilies=AF_INET AF_INET6
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
```

Включение и проверка:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now reflink
systemctl status reflink
curl http://127.0.0.1:8080/api/health   # {"status":"ok",...}
```

Служебные сообщения (запуск, ошибки конфига) идут в journald:
`journalctl -u reflink -f`. Лог **загрузок** — отдельный файл
`/var/log/reflink/uploads.log`.

## 5. Ротация лога (logrotate)

`/etc/logrotate.d/reflink`:

```
/var/log/reflink/uploads.log {
    weekly
    rotate 8
    compress
    missingok
    notifempty
    copytruncate
}
```

## 6. Caddy (HTTPS, reverse proxy)

Официальный apt-репозиторий (актуальные команды —
https://caddyserver.com/docs/install):

```bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
  | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
  | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy
```

`/etc/caddy/Caddyfile`:

```
reflink.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Caddy сам получает и продлевает сертификат Let's Encrypt. На VDS ничего
дополнительно открывать для TLS не нужно — только 80/443 в файрволе.

## 7. Firewall (ufw)

```bash
sudo apt install -y ufw
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status
```

## 8. Финальная проверка

```bash
curl https://reflink.example.com/api/health
curl -F "file=@some-photo.jpg" https://reflink.example.com/api/analyze
sudo tail -f /var/log/reflink/uploads.log
```

Затем открыть `https://reflink.example.com` в браузере. В логе за прокси
должен появиться реальный IP клиента (проверка TRUST_PROXY).

## 9. Обновление версии

1. Пересобрать (§1).
2. `scp build/reflink-linux user@...:/tmp/reflink`
3. На сервере:

```bash
sudo systemctl stop reflink
sudo mv /tmp/reflink /opt/reflink/reflink
sudo chmod 755 /opt/reflink/reflink
# если менялись static/* — обновить их тоже
sudo systemctl start reflink
curl http://127.0.0.1:8080/api/health
```

## 10. Откат

Перед обновлением сохраняйте прошлый бинарник:

```bash
sudo cp /opt/reflink/reflink /opt/reflink/reflink.prev
```

Откат: вернуть `reflink.prev` на место и перезапустить сервис.

## 11. Диагностика

| Симптом | Что проверить |
|---|---|
| сервис падает при старте | `journalctl -u reflink -n 100` (обычно невалидный env) |
| в логе IP `127.0.0.1` | включён ли `TRUST_PROXY=1` в юните |
| HTTPS не выдаётся | домен резолвится в IP? порты 80/443 открыты? `journalctl -u caddy -n 100` |
| порт 8080 занят | `sudo ss -tlnp | grep 8080` |
| 405 при POST | запрос идёт не на `/api/analyze` |

## 12. Отладка без Caddy (временно)

Можно открыть порт 8080 и ходить напрямую вовсе без TLS:

```bash
sudo ufw allow 8080/tcp
```

**Не оставляйте так на проде**: без Caddy `TRUST_PROXY` надо отключать
(иначе клиент сам подделает `X-Forwarded-For`), и трафик не шифруется.
Caddy — единственный компонент с внешним доступом: он на 80/443, reflink
слушает только 127.0.0.1.
