#!/usr/bin/env bash
#
# Генерация комплекта иконок asledger из единого источника —
# assets/logo/asledger-logo.svg. Копии по дереву репозитория не создаются:
# web-интерфейс, manifest и мобильное приложение ссылаются на один каталог.
#
# Инструменты (зафиксированные версии):
#   rsvg-convert (librsvg 2.x) — растрирование SVG;
#   ImageMagick 7 (magick) — дополнение до квадрата и сборка ICO.
#
# Использование: scripts/gen-assets.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
logo_dir="$root/assets/logo"
svg="$logo_dir/asledger-logo.svg"

if [[ ! -f "$svg" ]]; then
  echo "нет исходного логотипа: $svg" >&2
  exit 1
fi

for tool in rsvg-convert magick; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "не найден инструмент $tool" >&2
    exit 1
  fi
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# square <размер> <файл> <фон: none|white> — растр, дополненный до квадрата.
square() {
  local size="$1" out="$2" bg="$3"
  rsvg-convert -w "$size" -h "$size" --keep-aspect-ratio "$svg" >"$tmp/raw-$size.png"
  magick "$tmp/raw-$size.png" -background "$bg" -gravity center -extent "${size}x${size}" "$out"
}

square 16  "$logo_dir/favicon-16.png"  none
square 32  "$logo_dir/favicon-32.png"  none
square 48  "$logo_dir/favicon-48.png"  none
square 180 "$logo_dir/apple-touch-icon.png" white
square 192 "$logo_dir/icon-192.png" none
square 512 "$logo_dir/icon-512.png" none

# Многоразмерный favicon.ico: 16/32/48 в одном файле.
magick "$logo_dir/favicon-16.png" "$logo_dir/favicon-32.png" "$logo_dir/favicon-48.png" \
  "$logo_dir/favicon.ico"

# Совместимый растровый логотип для README и внешних ссылок.
square 224 "$logo_dir/asledger-favicon.png" none

cat >"$logo_dir/manifest.webmanifest" <<'JSON'
{
  "name": "asledger",
  "short_name": "asledger",
  "description": "Домашняя бухгалтерия с двойной записью",
  "lang": "ru",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f7f8f5",
  "theme_color": "#145214",
  "icons": [
    { "src": "/assets/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/assets/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable" }
  ]
}
JSON

echo "иконки обновлены в $logo_dir"
