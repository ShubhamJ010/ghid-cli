#!/bin/bash

SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/cli.sh"
CMD="$1"
shift || true

case "$CMD" in
    start)
        echo "🚀 Starting GhostHID background service..."
        adb shell am start-foreground-service -n com.poc.hiddebug/.HidService > /dev/null
        sleep 1.5
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || true
        ;;
    stop)
        echo "🛑 Stopping GhostHID background service and freeing resources..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd stop > /dev/null
        adb shell am stopservice -n com.poc.hiddebug/.HidService > /dev/null
        echo "Service stopped. Bluetooth HID freed."
        ;;
    lock)
        echo "⚡ Sending Win + L lock command..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_LOCK > /dev/null
        echo "Done."
        ;;
    sleep)
        echo "💤 Sending Windows Sleep sequence (Win+X -> u -> s)..."
        "$SCRIPT_PATH" combo "win+x"
        sleep 0.6
        "$SCRIPT_PATH" combo "u"
        sleep 0.4
        "$SCRIPT_PATH" combo "s"
        echo "Done."
        ;;
    type)
        TEXT="$*"
        if [ -z "$TEXT" ]; then
            echo "Usage: ./cli.sh type <text>"
            exit 1
        fi
        echo "⌨️ Typing text: $TEXT"
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd type --es text "$TEXT" > /dev/null
        echo "Done."
        ;;
    combo)
        KEYS="$1"
        if [ -z "$KEYS" ]; then
            echo "Usage: ./cli.sh combo <e.g. win+d, ctrl+alt+del, alt+tab>"
            exit 1
        fi
        echo "🎹 Sending key combination: $KEYS"
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd combo --es keys "$KEYS" > /dev/null
        echo "Done."
        ;;
    connect)
        echo "🔗 Connecting to laptop..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd connect > /dev/null
        sleep 2
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || true
        ;;
    disconnect)
        echo "🔌 Disconnecting..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd disconnect > /dev/null
        echo "Done."
        ;;
    status)
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || echo "Status file not found"
        ;;
    *)
        echo "Usage: $0 {start|stop|lock|sleep|type <text>|combo <keys>|connect|disconnect|status}"
        exit 1
        ;;
esac
