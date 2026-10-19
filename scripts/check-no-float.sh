#!/usr/bin/env bash
#
# Проверяет отсутствие типов с плавающей точкой в денежном коде.
#
# Правило: денежная арифметика ведётся только целыми копейками. Пакет
# pkg/core/money обязан обходиться без float32/float64, а файлы, работающие
# с денежным типом money.Kop, не должны использовать эти типы в коде.
#
# Комментарии не учитываются. Осознанно обойти правило можно пометкой
# «//nolint:nofloat» в той же строке.
set -euo pipefail

pattern='float32|float64'
status=0

# check_line разбирает строку вывода grep «файл:строка:текст» (или «строка:текст»)
# и сообщает о найденном нарушении, пропуская комментарии.
check_line() {
	local match="$1" reason="$2"
	local content trimmed
	content=$(printf '%s' "$match" | sed -E 's/^[^:]*:[0-9]+://')
	trimmed=$(printf '%s' "$content" | sed -E 's/^[[:space:]]+//')

	# Пропускаем строки комментариев: // …, /* … и * …
	case "$trimmed" in
	"//"*|"/*"*|"*"*) return 0 ;;
	esac
	# Пропускаем явное исключение.
	case "$content" in
	*"//nolint:nofloat"*) return 0 ;;
	esac

	echo "check-no-float: $reason" >&2
	echo "  $match" >&2
	status=1
}

# 1. Пакет money: типы с плавающей точкой запрещены полностью.
while IFS= read -r match; do
	[ -z "$match" ] && continue
	check_line "$match" "типы с плавающей точкой запрещены в pkg/core/money"
done < <(grep -RInE "$pattern" pkg/core/money --include='*.go' || true)

# 2. Файлы, которые ссылаются на денежный тип money.Kop.
while IFS= read -r file; do
	[ -z "$file" ] && continue
	case "$file" in
	./pkg/core/money/*) continue ;;
	esac
	while IFS= read -r match; do
		[ -z "$match" ] && continue
		check_line "$match" "типы с плавающей точкой рядом с денежной арифметикой в $file"
	done < <(grep -nE "$pattern" "$file" || true)
done < <(grep -RIl --include='*.go' 'money\.Kop' . 2>/dev/null || true)

if [ "$status" -eq 0 ]; then
	echo "check-no-float: типы с плавающей точкой в денежном коде не найдены"
fi
exit "$status"
