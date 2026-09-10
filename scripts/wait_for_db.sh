#!/usr/bin/env bash
set -euo pipefail

HOST="${1:?usage: wait_for_db.sh <host> <port> [timeout_seconds]}"
PORT="${2:?usage: wait_for_db.sh <host> <port> [timeout_seconds]}"
TIMEOUT="${3:-60}"

echo "Waiting up to ${TIMEOUT}s for ${HOST}:${PORT} ..."

for i in $(seq 1 "${TIMEOUT}"); do
  if (echo > "/dev/tcp/${HOST}/${PORT}") >/dev/null 2>&1; then
    echo "${HOST}:${PORT} is accepting connections (after ${i}s)."
    exit 0
  fi
  sleep 1
done

echo "ERROR: ${HOST}:${PORT} not ready within ${TIMEOUT}s." >&2
exit 1