#!/usr/bin/env bash
export LC_ALL=C
set -u

DB=/tmp/crashtest.db
ITERATIONS=20

rm -f "$DB" "$DB.wal"

echo "=== Тест восстановления после сбоя: $ITERATIONS циклов ==="

go build -o /tmp/crashtest ./cmd/crashtest || exit 1

for i in $(seq 1 $ITERATIONS); do
    /tmp/crashtest "$DB" fill > /dev/null 2>&1 &
    PID=$!

    SLEEP=$(LC_ALL=C awk "BEGIN {srand(); printf \"%.3f\", 0.05 + rand() * 0.45}")
    sleep "$SLEEP"

    kill -9 $PID 2>/dev/null
    wait $PID 2>/dev/null

    if ! /tmp/crashtest "$DB" verify; then
        echo "!!! ЦИКЛ $i: ПОВРЕЖДЕНИЕ ПОСЛЕ kill -9 (спал ${SLEEP}s)"
        exit 1
    fi

    echo "цикл $i: ok (спал ${SLEEP}s)"
done

echo "=== Все $ITERATIONS циклов пройдены. Дерево пережило kill -9. ==="