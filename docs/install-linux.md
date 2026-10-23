# Установка на Linux (systemd)

asledger работает как служба systemd: демон `asledgerd` поднимает
web-интерфейс на порту **8777** и REST API на **8778**, книга хранится в
SQLCipher (SQLite + шифрование, WAL) и является единственной точкой записи.

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

## 3. Пароль и шифрование книги

Книга шифруется SQLCipher. Пароль не хранится в `asledger.env`, argv или
`config.json`; рядом с книгой лежит только sidecar `asledger.db.kdf.json` с
параметрами Argon2id и солью. Для службы используйте systemd credentials.

Если книга осталась от `v1.2.0` (plaintext), сначала остановите службу и
зашифруйте её интерактивно от пользователя `asledger`:

```bash
sudo systemctl stop asledgerd
sudo -u asledger asledgerctl migrate-encrypt -db /var/lib/asledger/asledger.db
```

Команда проверит книгу, создаст зашифрованную резервную копию
`asledger.db.encrypted-backup.db`, sidecar и только затем заменит книгу.
Пароль вводится дважды в TTY. Для неинтерактивного запуска используйте
`-password-file` (файл `0600`) или `-password-fd`; `-dry-run` проверяет книгу
без изменений.

Создайте credential-файл, доступный только root:

```bash
sudo bash -c 'umask 077; read -rsp "Пароль книги: " p; printf "%s\n" "$p" > /etc/asledger/asledger-password; unset p'
```

В `/etc/systemd/system/asledgerd.service` раскомментируйте:

```ini
LoadCredential=asledger-password:/etc/asledger/asledger-password
```

`asledgerd` сам прочитает `$CREDENTIALS_DIRECTORY/asledger-password`.
Юнит также задаёт `LimitCORE=0` и `UMask=0077`, чтобы ключ не попадал в core
dump и файлы, доступные группе. Если credential недоступен, служба завершится
с ошибкой; автоматического отката к plaintext нет.

Смена пароля (службу остановить, старую книгу и sidecar сохранить):

```bash
sudo systemctl stop asledgerd
sudo -u asledger asledgerctl passwd -db /var/lib/asledger/asledger.db
sudo systemctl start asledgerd
```

## 4. Юнит systemd

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

## 5. Брандмауэр и доступ из локальной сети

Демон слушает все интерфейсы; для доступа с телефона в домашней сети откройте
порты:

```bash
sudo ufw allow 8777/tcp
sudo ufw allow 8778/tcp
```

Не выставляйте порты в интернет: аутентификация рассчитана на доверенную
домашнюю сеть.

## 6. Обновление

```bash
sudo systemctl stop asledgerd
sudo cp asledgerd asledgerctl /usr/local/bin/
sudo cp systemd/asledgerd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start asledgerd
```

Схема книги мигрируется автоматически при старте демона. Перед обновлением
сделайте резервную копию; команда спросит пароль в TTY и скопирует sidecar
рядом с копией:

```bash
sudo -u asledger asledgerctl backup -db /var/lib/asledger/asledger.db -out /var/lib/asledger/backup.db
```

Резервная копия зашифрована тем же ключом и без `backup.db.kdf.json`
не откроется. Храните её вместе с sidecar.

## 7. Журнал и диагностика

```bash
journalctl -u asledgerd -f
asledgerctl version
asledgerd --version
```

Если служба не стартует, проверьте права на `/var/lib/asledger`, наличие
`LoadCredential` и что каталоги `assets`/`templates` доступны пользователю
`asledger`. Ошибки «нужен пароль», «sidecar не найден» и «неверный ключ»
пишутся в journal без самого пароля/ключа.
