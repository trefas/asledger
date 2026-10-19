#!/usr/bin/env bash
#
# Проверяет карту трассировки «критерий раздела 14 → автотест»
# (документ docs/traceability.md).
#
# Правила:
#   1. Каждая строка критерия начинается с «| K-» и содержит идентификатор,
#      текст и колонку тестов.
#   2. Колонка тестов — непустой список имён, разделённых запятыми; каждое имя
#      должно существовать в репозитории (файлы *_test.go).
#   3. Допускается отложенный критерий: колонка начинается со слова «план» и
#      ссылается на задачу «#N» (например, «план: #86»). Такой критерий не
#      считается покрытым и не роняет проверку, но виден в отчёте.
#   4. Набор критериев — K-01 … K-19 без пропусков и повторов.
#
# Скрипт запускается в CI (make check) и локально. Любое нарушение — ненулевой код.

set -euo pipefail

map_file="${1:-docs/traceability.md}"
if [ ! -f "$map_file" ]; then
	echo "проверка трассировки: не найден файл $map_file" >&2
	exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

tests_file="$work_dir/tests.txt"
grep -rhoE '^func (Test[A-Za-z0-9_]+)\(' --include='*_test.go' . |
	sed -E 's/^func //; s/\($//' | sort -u >"$tests_file"

criteria_file="$work_dir/criteria.txt"
grep -E '^\| K-[0-9]+ ' "$map_file" >"$criteria_file" || true

fail=0
declared=0
covered=0
deferred=0

while IFS= read -r line; do
	id="$(printf '%s' "$line" | awk -F'|' '{gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}')"
	tests="$(printf '%s' "$line" | awk -F'|' '{print $4}')"
	tests="$(printf '%s' "$tests" | sed -E 's/^[ \t]+|[ \t]+$//g')"
	declared=$((declared + 1))

	if [ -z "${tests// /}" ]; then
		echo "критерий $id: не указан автотест" >&2
		fail=1
		continue
	fi

	if [[ "$tests" == план* ]]; then
		if ! printf '%s' "$tests" | grep -qE '#[0-9]+'; then
			echo "критерий $id: отложенный критерий должен ссылаться на задачу «#N»" >&2
			fail=1
			continue
		fi
		deferred=$((deferred + 1))
		echo "критерий $id: отложен ($tests)"
		continue
	fi

	covered=$((covered + 1))
	IFS=',' read -ra list <<<"$tests"
	for raw in "${list[@]}"; do
		name="$(printf '%s' "$raw" | sed -E 's/^[ \t]+|[ \t]+$//g')"
		if [ -z "$name" ]; then
			echo "критерий $id: пустое имя теста в списке «$tests»" >&2
			fail=1
			continue
		fi
		if ! printf '%s' "$name" | grep -qE '^TestASL_[0-9]+_'; then
			echo "критерий $id: имя «$name» не похоже на трассируемый тест" >&2
			fail=1
		fi
		if ! grep -qx "$name" "$tests_file"; then
			echo "критерий $id: тест «$name» не найден в репозитории" >&2
			fail=1
		fi
	done
done <"$criteria_file"

# Полнота и уникальность набора критериев.
missing=""
for n in $(seq -w 1 19); do
	if ! grep -qE "^\| K-$n " "$criteria_file"; then
		missing="$missing K-$n"
	fi
done
if [ -n "$missing" ]; then
	echo "в карте нет критериев:$missing" >&2
	fail=1
fi
if [ "$(sort -u "$criteria_file" | wc -l)" -ne "$declared" ]; then
	echo "в карте есть повторяющиеся критерии" >&2
	fail=1
fi

if [ "$fail" -ne 0 ]; then
	echo "проверка трассировки: ошибки (критериев $declared, покрыто $covered, отложено $deferred)" >&2
	exit 1
fi

echo "проверка трассировки: критериев $declared, покрыто $covered, отложено $deferred"
