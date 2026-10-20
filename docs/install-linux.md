# Установка на Linux (systemd)

asledger работает как служба systemd: демон `asledgerd` поднимает
web-интерфейс на порту **8777** и REST API на **8778**, книга хранится в
SQLite (WAL) и является единственной точкой записи.

## 1. Распаковка архива

Релизный архив `asledger_<версия>_linux_<арх>.tar.gz` содержит:

```
asledgerd               демон (web + API)
asledgerctl             утилита обслуживания книги
docs/                   документация
systemd/asledgerd.service
systemd/asledgerd.env
templates/charts/       шаблонные планы счетов
assets/logo/            лого и favicon
README.md, LICENSE
```

## 2. Пользователь и каталоги

```bash
sudo useradd --system --home /var/lib/asledger --shell /usr/sbin/nologin asledger
sudo mkdir -p /var/lib/asledger /etc/asledger /usr/local/share/asledger
sudo cp asledgerd asledgerctl /usr/local/bin/
sudo cp -r templates/charts /usr/local/share/asledger/templates/
sudo cp -r assets/logo /usr/local/share/asledger/assets/
sudo chown -R asledger:asledger /var/lib/asledger
sudo chmod 750 /var/lib/asledger
```

## 3. Юнит systemd

```bash
sudo cp systemd/asledgerd.service /etc/systemd/system/
sudo cp systemd/asledgerd.env /etc/asledger/asledger.env
sudo systemctl daemon-reload
sudo systemctl enable --now asledgerd
```

Проверка:

```bash
systemctl status asledgerd
curl -fsS http://localhost:8778/api/v1/health
```

Порты по умолчанию — `:8777` (web) и `:8778` (API). Изменить их можно флагами
`-web-addr`/`-api-addr` в `ExecStart` или переменными окружения в
`/etc/asledger/asledger.env`.

## 4. Брандмауэр и доступ из локальной сети

Демон слушает все интерфейсы; для доступа с телефона в домашней сети откройте
порты:

```bash
sudo ufw allow 8777/tcp
sudo ufw allow 8778/tcp
```

Не выставляйте порты в интернет: аутентификация рассчитана на доверенную
домашнюю сеть.

## 5. Обновление

```bash
sudo systemctl stop asledgerd
sudo cp asledgerd asledgerctl /usr/local/bin/
sudo cp systemd/asledgerd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start asledgerd
```

Схема книги мигрируется автоматически при старте демона. Перед обновлением
сделайте резервную копию:

```bash
sudo -u asledger asledgerctl backup -db /var/lib/asledger/asledger.db -out /var/lib/asledger/backup.db
```

## 6. Журнал и диагностика

```bash
journalctl -u asledgerd -f
asledgerctl version
asledgerd --version
```

Если служба не стартует, проверьте права на `/var/lib/asledger` и что каталоги
`assets`/`templates` доступны пользователю `asledger`.
