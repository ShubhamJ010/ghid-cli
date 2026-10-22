#!/bin/bash

CMD="$1"
shift || true

case "$CMD" in
    lock)
        echo "⚡ Sending Win + L lock command..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_LOCK > /dev/null
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
        echo "Usage: $0 {lock|type <text>|combo <keys>|connect|disconnect|status}"
        exit 1
        ;;
esac
