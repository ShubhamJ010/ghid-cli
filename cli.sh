#!/bin/bash

SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/cli.sh"
CMD="$1"
shift || true

case "$CMD" in
    start)
        echo "🚀 Launching GhostHID Headless Gateway..."
        adb shell am start-foreground-service -n com.poc.hiddebug/.HidService > /dev/null
        sleep 1.5
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || true
        ;;
    stop)
        echo "🛑 Stopping GhostHID Gateway and releasing Bluetooth resources..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd stop > /dev/null
        adb shell am stopservice -n com.poc.hiddebug/.HidService > /dev/null
        echo "Service stopped. Zero resources active."
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
    raw)
        HEX="$1"
        if [ -z "$HEX" ] || [ "${#HEX}" -ne 16 ]; then
            echo "Usage: ./cli.sh raw <16-hex-chars>  (e.g. 08000f0000000000 for Win+L)"
            exit 1
        fi
        echo "📦 Sending raw 8-byte HID report: $HEX"
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd raw --es hex "$HEX" > /dev/null
        echo "Done."
        ;;
    connect)
        ADDR="$1"
        echo "🔗 Connecting to target laptop..."
        if [ -n "$ADDR" ]; then
            adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd connect --es address "$ADDR" > /dev/null
        else
            adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd connect > /dev/null
        fi
        sleep 2
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || true
        ;;
    disconnect)
        echo "🔌 Disconnecting..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd disconnect > /dev/null
        echo "Done."
        ;;
    unpair)
        echo "🗑️ Unpairing target laptop..."
        adb shell am broadcast -n com.poc.hiddebug/.HidCommandReceiver -a com.poc.hiddebug.ACTION_CMD --es cmd unpair > /dev/null
        echo "Done."
        ;;
    status)
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/status.json 2>/dev/null || echo "Status file not found"
        ;;
    logs)
        LINES="${1:-35}"
        echo "📜 Showing last $LINES log lines from GhostHID:"
        adb logcat -d | grep -i "GHOST_HID" | tail -n "$LINES"
        ;;
    tail)
        echo "📡 Streaming live GhostHID logs (Ctrl+C to stop)..."
        adb logcat -v time | grep --line-buffered -i "GHOST_HID"
        ;;
    filelog)
        adb shell cat /sdcard/Android/data/com.poc.hiddebug/files/gateway.log 2>/dev/null || echo "Log file empty or not found"
        ;;
    *)
        echo "GhostHID Headless Gateway CLI"
        echo "Usage: $0 {start|stop|lock|sleep|type <text>|combo <keys>|raw <hex>|connect|disconnect|unpair|status|logs [n]|tail|filelog}"
        exit 1
        ;;
esac
