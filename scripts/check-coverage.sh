#!/usr/bin/env bash
#
# Проверяет общий порог покрытия тестами (раздел 13.1 эпика: «покрытие по порогу»).
#
# Порог задаётся переменной окружения COVERAGE_MIN (по умолчанию 70 %).
# Запускается в CI; локально — целью make cover-check.
set -euo pipefail

threshold="${COVERAGE_MIN:-70}"
profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

go test -coverprofile="$profile" ./... >/dev/null
total="$(go tool cover -func="$profile" | tail -n 1 | awk '{print $3}' | tr -d '%')"

if [ -z "$total" ]; then
	echo "check-coverage: не удалось измерить покрытие" >&2
	exit 1
fi

if awk -v got="$total" -v need="$threshold" 'BEGIN { exit !(got + 0 >= need + 0) }'; then
	echo "check-coverage: покрытие ${total}% ≥ порога ${threshold}%"
else
	echo "check-coverage: покрытие ${total}% ниже порога ${threshold}%" >&2
	exit 1
fi
