#!/bin/bash
set -u

worker_pid=""
api_pid=""

shutdown() {
    status="${1:-0}"

    trap - INT TERM

    if [ -n "$worker_pid" ]; then
        kill -TERM "$worker_pid" 2>/dev/null || true
    fi

    if [ -n "$api_pid" ]; then
        kill -TERM "$api_pid" 2>/dev/null || true
    fi

    wait "$worker_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true

    exit "$status"
}

trap 'shutdown 143' TERM
trap 'shutdown 130' INT

/usr/local/bin/coffee-reel-worker &
worker_pid=$!

/usr/local/bin/coffee-reel-api &
api_pid=$!

wait -n "$worker_pid" "$api_pid"
status=$?

shutdown "$status"
