#!/usr/bin/env bash
# Генерация Android-иконок из единого логотипа assets/logo/icon-512.png.
#
# Скрипт нужен при обновлении логотипа; готовые PNG лежат в репозитории, чтобы
# CI не зависел от ImageMagick. Запуск: android/scripts/gen-icons.sh.
set -euo pipefail
cd "$(dirname "$0")/.."

src="../assets/logo/icon-512.png"
res="app/src/main/res"

mkdir -p "$res/drawable-nodpi"
cp "$src" "$res/drawable-nodpi/asledger_logo.png"

# Плотности launcher-иконок: 48/72/96/144/192 px для legacy.
densities=(mdpi hdpi xhdpi xxhdpi xxxhdpi)
legacy_sizes=(48 72 96 144 192)

for i in "${!densities[@]}"; do
    density="${densities[$i]}"
    size="${legacy_sizes[$i]}"
    dir="$res/mipmap-$density"
    mkdir -p "$dir"

    inner=$((size * 70 / 100))
    convert -size "${size}x${size}" xc:'#F7F8F5' \
        \( "$src" -resize "${inner}x${inner}" \) -gravity center -composite \
        "$dir/ic_launcher.png"

    radius=$((size / 2))
    convert -size "${size}x${size}" xc:none \
        -fill '#F7F8F5' -draw "circle $radius,$radius $radius,0" \
        \( "$src" -resize "${inner}x${inner}" \) -gravity center -composite \
        "$dir/ic_launcher_round.png"

    # Adaptive foreground: 108dp, логотип внутри safe zone 66dp.
    fg=$((size * 108 / 48))
    fg_inner=$((fg * 58 / 100))
    convert -size "${fg}x${fg}" xc:none \
        \( "$src" -resize "${fg_inner}x${fg_inner}" \) -gravity center -composite \
        "$dir/ic_launcher_foreground.png"
done

echo "иконки Android обновлены"
