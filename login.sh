#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI="$SCRIPT_DIR/cli.sh"

USERNAME="$1"
PASSWORD="$2"

if [ -z "$USERNAME" ]; then
    read -p "Enter Windows Username: " USERNAME
fi

if [ -z "$PASSWORD" ]; then
    read -s -p "Enter Windows Password: " PASSWORD
    echo ""
fi

echo "=== GhostHID Windows Auto-Login Sequence ==="

echo "1. Waking lock screen (Ctrl + Alt + Delete)..."
"$CLI" combo "ctrl+alt+del"
sleep 1

echo "2. Clearing lock overlay (Escape)..."
"$CLI" combo "esc"
sleep 1

echo "3. Typing username..."
"$CLI" type "$USERNAME"
sleep 1.5

echo "4. Navigating to password field (Tab)..."
"$CLI" combo "tab"
sleep 1

echo "5. Typing password..."
"$CLI" type "$PASSWORD"
sleep 1.5

echo "6. Submitting (Enter)..."
"$CLI" combo "enter"
sleep 1

echo "=== Login sequence completed ==="
