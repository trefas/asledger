# Установка на Windows

Релизный архив `asledger_<версия>_windows_amd64.zip` содержит `asledgerd.exe`,
`asledgerctl.exe`, каталоги `templates\charts`, `assets\logo`, документацию и
`README.md`.

Демон можно запускать в консоли или как службу Windows (через NSSM либо
`sc.exe`). Порты по умолчанию — **8777** (web) и **8778** (API).

## 1. Распаковка

Распакуйте архив, например в `C:\asledger`:

```
C:\asledger\asledgerd.exe
C:\asledger\asledgerctl.exe
C:\asledger\templates\charts\
C:\asledger\assets\logo\
```

Книга по умолчанию создаётся в рабочем каталоге как `asledger.db` и
шифруется SQLCipher. Удобно держать данные отдельно:

```
C:\ProgramData\asledger\asledger.db
```

## 2. Пароль и шифрование

При первом консольном запуске `asledgerd` спросит пароль и создаст sidecar
`asledger.db.kdf.json` с параметрами Argon2id. Пароль не хранится в
`config.json`, argv или переменных окружения. Старую plaintext-книгу
`v1.2.0` зашифруйте явно:

```powershell
.\asledgerctl.exe migrate-encrypt -db C:\ProgramData\asledger\asledger.db
```

Команда создаст зашифрованную резервную копию
`asledger.db.encrypted-backup.db`, проверит её и заменит книгу. `-dry-run`
проверяет без изменений; `-password-file`/`-password-fd` подходят для
неинтерактивного запуска.

Для службы TTY нет, поэтому передайте пароль через файл, доступный только
службе. Создайте `C:\ProgramData\asledger\asledger-password` и ограничьте
ACL:

```powershell
$path = "C:\ProgramData\asledger\asledger-password"
# Введите пароль в защищённом окне; не сохраняйте его в истории PowerShell.
$secure = Read-Host "Пароль книги" -AsSecureString
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
  $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
  [IO.File]::WriteAllText($path, $plain + "`n")
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
  Remove-Variable plain -ErrorAction SilentlyContinue
}
icacls $path /inheritance:r /grant:r "NT AUTHORITY\SYSTEM:R" "BUILTIN\Administrators:F"
```

Путь к файлу не является секретом; содержимое — секрет. Для службы
используйте `-password-file`; автоматического отката к plaintext нет.

## 3. Консольный запуск

Откройте PowerShell:

```powershell
cd C:\asledger
.\asledgerd.exe `
  -db C:\ProgramData\asledger\asledger.db `
  -assets C:\asledger\assets\logo `
  -charts-dir C:\asledger\templates\charts `
  -web-addr :8777 -api-addr :8778
```

Проверка: `http://localhost:8777` и `http://localhost:8778/api/v1/health`.

## 4. Служба через NSSM

[NSSM](https://nssm.cc/) удобен тем, что перезапускает процесс и пишет журнал.

```powershell
nssm install asledger "C:\asledger\asledgerd.exe"
nssm set asledger AppDirectory "C:\ProgramData\asledger"
nssm set asledger AppParameters "-db C:\ProgramData\asledger\asledger.db -assets C:\asledger\assets\logo -charts-dir C:\asledger\templates\charts -web-addr :8777 -api-addr :8778 -password-file C:\ProgramData\asledger\asledger-password"
nssm set asledger AppStdout "C:\ProgramData\asledger\asledger.log"
nssm set asledger AppStderr "C:\ProgramData\asledger\asledger.err.log"
nssm set asledger Start SERVICE_AUTO_START
nssm start asledger
```

Обновление: остановить службу (`nssm stop asledger`), заменить `.exe`, запустить
снова.

## 5. Служба через sc.exe

`sc.exe` не умеет запускать обычные консольные программы напрямую, поэтому
используется обёртка. Пример с `cmd.exe`:

```powershell
sc.exe create asledger binPath= "cmd.exe /c C:\asledger\asledgerd.exe -db C:\ProgramData\asledger\asledger.db -assets C:\asledger\assets\logo -charts-dir C:\asledger\templates\charts -web-addr :8777 -api-addr :8778 -password-file C:\ProgramData\asledger\asledger-password >> C:\ProgramData\asledger\asledger.log 2>&1" start= auto
sc.exe description asledger "asledger — домашняя бухгалтерия"
sc.exe start asledger
```

Для остановки — `sc.exe stop asledger`. Обёртка не перезапускает процесс при
падении; для постоянной службы предпочтителен NSSM.

## 6. Брандмауэр

Разрешите входящие подключения на портах 8777 и 8778 в локальной сети:

```powershell
New-NetFirewallRule -DisplayName "asledger web" -Direction Inbound -Protocol TCP -LocalPort 8777 -Action Allow
New-NetFirewallRule -DisplayName "asledger api" -Direction Inbound -Protocol TCP -LocalPort 8778 -Action Allow
```

Не открывайте порты в интернет: доступ рассчитан на доверенную домашнюю сеть.

## 7. Резервная копия и диагностика

```powershell
.\asledgerctl.exe backup -db C:\ProgramData\asledger\asledger.db -out C:\ProgramData\asledger\backup.db
.\asledgerctl.exe version
.\asledgerd.exe --version
```

`backup` спросит пароль и скопирует рядом `backup.db.kdf.json`; без sidecar
копия не откроется. Смена пароля: остановите службу и выполните
`asledgerctl passwd -db ...` (старая книга и sidecar сохраняются как
`asledger.db.pre-rekey.db` и `.kdf.json`).

Конфликт портов (`bind: address already in use`) означает, что порт занят другой
программой: проверьте `netstat -ano | findstr :8777` и задайте другие адреса
флагами `-web-addr`/`-api-addr`. Если служба пишет «нужен пароль» или
«sidecar не найден», проверьте `-password-file`, ACL файла и наличие
`asledger.db.kdf.json` рядом с книгой.

Пути с пробелами заключайте в кавычки в параметрах службы.
