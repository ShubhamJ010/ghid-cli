package com.poc.hiddebug;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothHidDevice;
import android.bluetooth.BluetoothHidDeviceAppSdpSettings;
import android.bluetooth.BluetoothProfile;
import android.content.Context;
import android.content.Intent;
import android.os.Binder;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;

import java.io.File;
import java.io.FileWriter;
import java.lang.reflect.Method;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class HidService extends Service {

    public static final String TAG = "HID_DEBUG";
    private static final String CHANNEL_ID = "hid_service_channel";

    public static final String ACTION_CMD = "com.poc.hiddebug.ACTION_CMD";
    public static final String ACTION_LOCK = "com.poc.hiddebug.ACTION_LOCK";
    public static final String ACTION_TYPE = "com.poc.hiddebug.ACTION_TYPE";

    private static final byte[] HID_KEYBOARD_DESCRIPTOR = new byte[] {
        (byte) 0x05, (byte) 0x01,        // Usage Page (Generic Desktop)
        (byte) 0x09, (byte) 0x06,        // Usage (Keyboard)
        (byte) 0xA1, (byte) 0x01,        // Collection (Application)
        (byte) 0x85, (byte) 0x01,        //   Report ID (1)
        (byte) 0x05, (byte) 0x07,        //   Usage Page (Key Codes)
        (byte) 0x19, (byte) 0xE0,        //   Usage Minimum (224)
        (byte) 0x29, (byte) 0xE7,        //   Usage Maximum (231)
        (byte) 0x15, (byte) 0x00,        //   Logical Minimum (0)
        (byte) 0x25, (byte) 0x01,        //   Logical Maximum (1)
        (byte) 0x75, (byte) 0x01,        //   Report Size (1)
        (byte) 0x95, (byte) 0x08,        //   Report Count (8)
        (byte) 0x81, (byte) 0x02,        //   Input (Data, Variable, Absolute) - Modifiers
        (byte) 0x95, (byte) 0x01,        //   Report Count (1)
        (byte) 0x75, (byte) 0x08,        //   Report Size (8)
        (byte) 0x81, (byte) 0x01,        //   Input (Constant) - Reserved
        (byte) 0x95, (byte) 0x05,        //   Report Count (5)
        (byte) 0x75, (byte) 0x01,        //   Report Size (1)
        (byte) 0x05, (byte) 0x08,        //   Usage Page (LEDs)
        (byte) 0x19, (byte) 0x01,        //   Usage Minimum (1)
        (byte) 0x29, (byte) 0x05,        //   Usage Maximum (5)
        (byte) 0x91, (byte) 0x02,        //   Output (Data, Variable, Absolute) - LEDs
        (byte) 0x95, (byte) 0x01,        //   Report Count (1)
        (byte) 0x75, (byte) 0x03,        //   Report Size (3)
        (byte) 0x91, (byte) 0x01,        //   Output (Constant) - LED padding
        (byte) 0x95, (byte) 0x06,        //   Report Count (6)
        (byte) 0x75, (byte) 0x08,        //   Report Size (8)
        (byte) 0x15, (byte) 0x00,        //   Logical Minimum (0)
        (byte) 0x25, (byte) 0x65,        //   Logical Maximum (101)
        (byte) 0x05, (byte) 0x07,        //   Usage Page (Key Codes)
        (byte) 0x19, (byte) 0x00,        //   Usage Minimum (0)
        (byte) 0x29, (byte) 0x65,        //   Usage Maximum (101)
        (byte) 0x81, (byte) 0x00,        //   Input (Data, Array) - Keys
        (byte) 0xC0                      // End Collection
    };

    public interface StateListener {
        void onLog(String log);
        void onStateUpdated();
    }

    private static HidService sInstance;
    private final IBinder mBinder = new LocalBinder();
    private final List<StateListener> mListeners = new ArrayList<>();

    private BluetoothAdapter mAdapter;
    private BluetoothHidDevice mHidDevice;
    private BluetoothDevice mTargetDevice;
    private boolean mIsRegistered = false;
    private int mConnectionState = BluetoothProfile.STATE_DISCONNECTED;

    private ExecutorService mExecutor;
    private Handler mHandler;
    private SimpleDateFormat mTimeFormat;

    public class LocalBinder extends Binder {
        public HidService getService() {
            return HidService.this;
        }
    }

    public static HidService getInstance() {
        return sInstance;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        sInstance = this;
        mExecutor = Executors.newSingleThreadExecutor();
        mHandler = new Handler(Looper.getMainLooper());
        mTimeFormat = new SimpleDateFormat("HH:mm:ss.SSS", Locale.US);

        createNotificationChannel();
        startForeground(1001, buildNotification("HID Service Running"));

        log("HidService created.");
        initBluetooth();
    }

    private void createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID,
                "HID Background Service",
                NotificationManager.IMPORTANCE_LOW
            );
            channel.setDescription("Maintains Bluetooth HID connection in the background");
            NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
            if (nm != null) {
                nm.createNotificationChannel(channel);
            }
        }
    }

    private Notification buildNotification(String text) {
        Notification.Builder builder;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            builder = new Notification.Builder(this, CHANNEL_ID);
        } else {
            builder = new Notification.Builder(this);
        }
        return builder
            .setContentTitle("HID Debug Service")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setOngoing(true)
            .build();
    }

    private void updateNotification(String text) {
        NotificationManager nm = (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
        if (nm != null) {
            nm.notify(1001, buildNotification(text));
        }
    }

    public void addListener(StateListener l) {
        synchronized (mListeners) {
            if (!mListeners.contains(l)) mListeners.add(l);
        }
    }

    public void removeListener(StateListener l) {
        synchronized (mListeners) {
            mListeners.remove(l);
        }
    }

    private void initBluetooth() {
        mAdapter = BluetoothAdapter.getDefaultAdapter();
        if (mAdapter == null) {
            log("ERROR: Bluetooth not supported!");
            return;
        }
        log("Bluetooth enabled: " + mAdapter.isEnabled());
        mAdapter.getProfileProxy(this, mProfileListener, BluetoothProfile.HID_DEVICE);
    }

    private final BluetoothProfile.ServiceListener mProfileListener = new BluetoothProfile.ServiceListener() {
        @Override
        public void onServiceConnected(int profile, BluetoothProfile proxy) {
            if (profile == BluetoothProfile.HID_DEVICE) {
                mHidDevice = (BluetoothHidDevice) proxy;
                log("BluetoothHidDevice proxy connected.");
                registerHidApp();
                refreshDevices();
            }
        }

        @Override
        public void onServiceDisconnected(int profile) {
            if (profile == BluetoothProfile.HID_DEVICE) {
                mHidDevice = null;
                mIsRegistered = false;
                log("BluetoothHidDevice proxy disconnected.");
                notifyStateChanged();
            }
        }
    };

    public void registerHidApp() {
        if (mHidDevice == null) return;
        BluetoothHidDeviceAppSdpSettings sdp = new BluetoothHidDeviceAppSdpSettings(
            "OnePlus HID Keyboard",
            "Virtual HID Keyboard",
            "Android",
            BluetoothHidDevice.SUBCLASS1_KEYBOARD,
            HID_KEYBOARD_DESCRIPTOR
        );
        boolean ok = mHidDevice.registerApp(sdp, null, null, mExecutor, mHidCallback);
        log("registerApp() returned: " + ok);
    }

    private final BluetoothHidDevice.Callback mHidCallback = new BluetoothHidDevice.Callback() {
        @Override
        public void onAppStatusChanged(BluetoothDevice pluggedDevice, boolean registered) {
            mIsRegistered = registered;
            log("onAppStatusChanged: registered=" + registered);
            notifyStateChanged();
        }

        @Override
        public void onConnectionStateChanged(BluetoothDevice device, int state) {
            mConnectionState = state;
            log("onConnectionStateChanged: [" + (device != null ? device.getName() : "null") + "] -> " + stateToString(state));
            if (state == BluetoothProfile.STATE_CONNECTED) {
                mTargetDevice = device;
                updateNotification("Connected to " + device.getName());
            } else {
                updateNotification("Status: " + stateToString(state));
            }
            writeStatusFile();
            notifyStateChanged();
        }

        @Override
        public void onGetReport(BluetoothDevice device, byte type, byte id, int bufferSize) {
            log("onGetReport: type=" + type + ", id=" + id);
            if (mHidDevice != null) {
                mHidDevice.replyReport(device, type, id, new byte[8]);
            }
        }

        @Override
        public void onSetReport(BluetoothDevice device, byte type, byte id, byte[] data) {
            log("onSetReport: type=" + type + ", id=" + id);
            if (mHidDevice != null) {
                mHidDevice.replyReport(device, type, id, new byte[0]);
            }
        }
    };

    public void refreshDevices() {
        if (mAdapter == null) return;
        try {
            Set<BluetoothDevice> paired = mAdapter.getBondedDevices();
            if (paired != null) {
                for (BluetoothDevice d : paired) {
                    String name = d.getName();
                    String addr = d.getAddress();
                    if (name != null && (name.startsWith("LP-") || name.contains("TZD") || addr.equalsIgnoreCase("38:FC:98:E8:E3:87"))) {
                        mTargetDevice = d;
                        log("Target laptop selected: " + name + " [" + addr + "]");
                    }
                }
            }
        } catch (Exception e) {
            log("refreshDevices error: " + e.getMessage());
        }
        writeStatusFile();
        notifyStateChanged();
    }

    public void connectTarget() {
        if (mHidDevice == null || mTargetDevice == null) {
            refreshDevices();
        }
        if (mHidDevice != null && mTargetDevice != null) {
            log("Connecting to " + mTargetDevice.getName() + "...");
            boolean ok = mHidDevice.connect(mTargetDevice);
            log("connect() returned: " + ok);
        } else {
            log("Cannot connect: HID proxy or target is null");
        }
    }

    public void disconnectTarget() {
        if (mHidDevice != null && mTargetDevice != null) {
            log("Disconnecting from " + mTargetDevice.getName() + "...");
            mHidDevice.disconnect(mTargetDevice);
        }
    }

    public void sendWinL() {
        if (mHidDevice == null || mTargetDevice == null) {
            log("Cannot send Win+L: device not ready");
            return;
        }
        mExecutor.execute(() -> {
            log(">>> Executing Win+L lock sequence...");
            byte[] down = new byte[] { (byte) 0x08, 0x00, (byte) 0x0F, 0, 0, 0, 0, 0 };
            byte[] up = new byte[8];
            boolean r1 = mHidDevice.sendReport(mTargetDevice, 1, down);
            try { Thread.sleep(70); } catch (Exception ignored) {}
            boolean r2 = mHidDevice.sendReport(mTargetDevice, 1, up);
            log("Win+L down=" + r1 + ", up=" + r2);
        });
    }

    public void sendKey(byte modifier, byte keyCode, int durationMs) {
        if (mHidDevice == null || mTargetDevice == null) return;
        mExecutor.execute(() -> {
            byte[] down = new byte[] { modifier, 0, keyCode, 0, 0, 0, 0, 0 };
            byte[] up = new byte[8];
            mHidDevice.sendReport(mTargetDevice, 1, down);
            try { Thread.sleep(durationMs > 0 ? durationMs : 40); } catch (Exception ignored) {}
            mHidDevice.sendReport(mTargetDevice, 1, up);
            try { Thread.sleep(20); } catch (Exception ignored) {}
        });
    }

    public void typeText(String text) {
        if (mHidDevice == null || mTargetDevice == null || text == null) return;
        mExecutor.execute(() -> {
            log(">>> Typing text (" + text.length() + " chars): " + text);
            for (int i = 0; i < text.length(); i++) {
                char c = text.charAt(i);
                byte modifier = 0;
                byte code = 0;

                if (c >= 'a' && c <= 'z') {
                    code = (byte) (0x04 + (c - 'a'));
                } else if (c >= 'A' && c <= 'Z') {
                    modifier = 0x02; // Shift
                    code = (byte) (0x04 + (c - 'A'));
                } else if (c >= '1' && c <= '9') {
                    code = (byte) (0x1E + (c - '1'));
                } else if (c == '0') {
                    code = 0x27;
                } else if (c == ' ') {
                    code = 0x2C;
                } else if (c == '\n' || c == '\r') {
                    code = 0x28; // Enter
                } else if (c == '\t') {
                    code = 0x2B; // Tab
                } else if (c == '.') {
                    code = 0x37;
                } else if (c == ',') {
                    code = 0x36;
                } else if (c == '-') {
                    code = 0x2D;
                } else if (c == '_') {
                    modifier = 0x02; code = 0x2D;
                } else if (c == '!') {
                    modifier = 0x02; code = 0x1E;
                } else if (c == '@') {
                    modifier = 0x02; code = 0x1F;
                } else if (c == '#') {
                    modifier = 0x02; code = 0x20;
                } else if (c == '$') {
                    modifier = 0x02; code = 0x21;
                } else if (c == '%') {
                    modifier = 0x02; code = 0x22;
                } else if (c == ':') {
                    modifier = 0x02; code = 0x33;
                }

                if (code != 0) {
                    byte[] down = new byte[] { modifier, 0, code, 0, 0, 0, 0, 0 };
                    byte[] up = new byte[8];
                    mHidDevice.sendReport(mTargetDevice, 1, down);
                    try { Thread.sleep(25); } catch (Exception ignored) {}
                    mHidDevice.sendReport(mTargetDevice, 1, up);
                    try { Thread.sleep(25); } catch (Exception ignored) {}
                }
            }
            log(">>> Finished typing.");
        });
    }

    public void sendCombo(String combo) {
        if (mHidDevice == null || mTargetDevice == null || combo == null) return;
        mExecutor.execute(() -> {
            log(">>> Executing combo: " + combo);
            String[] parts = combo.toLowerCase(Locale.ROOT).split("\\+");
            byte modifier = 0;
            byte key = 0;

            for (String p : parts) {
                p = p.trim();
                if (p.equals("win") || p.equals("gui") || p.equals("cmd")) modifier |= 0x08;
                else if (p.equals("ctrl")) modifier |= 0x01;
                else if (p.equals("alt")) modifier |= 0x04;
                else if (p.equals("shift")) modifier |= 0x02;
                else if (p.equals("del") || p.equals("delete")) key = 0x4C;
                else if (p.equals("esc") || p.equals("escape")) key = 0x29;
                else if (p.equals("enter") || p.equals("return")) key = 0x28;
                else if (p.equals("tab")) key = 0x2B;
                else if (p.equals("space")) key = 0x2C;
                else if (p.length() == 1) {
                    char c = p.charAt(0);
                    if (c >= 'a' && c <= 'z') key = (byte) (0x04 + (c - 'a'));
                    else if (c >= '1' && c <= '9') key = (byte) (0x1E + (c - '1'));
                    else if (c == '0') key = 0x27;
                }
            }

            byte[] down = new byte[] { modifier, 0, key, 0, 0, 0, 0, 0 };
            byte[] up = new byte[8];
            mHidDevice.sendReport(mTargetDevice, 1, down);
            try { Thread.sleep(60); } catch (Exception ignored) {}
            mHidDevice.sendReport(mTargetDevice, 1, up);
            log("Combo sent: mod=" + modifier + ", key=" + key);
        });
    }

    public void unpairTarget() {
        if (mTargetDevice == null) refreshDevices();
        if (mTargetDevice != null) {
            try {
                log("Unpairing " + mTargetDevice.getName() + "...");
                Method m = mTargetDevice.getClass().getMethod("removeBond");
                m.invoke(mTargetDevice);
                mTargetDevice = null;
                mConnectionState = BluetoothProfile.STATE_DISCONNECTED;
                writeStatusFile();
                notifyStateChanged();
            } catch (Exception e) {
                log("Unpair error: " + e.getMessage());
            }
        }
    }

    private void writeStatusFile() {
        try {
            File f = new File(getExternalFilesDir(null), "status.json");
            FileWriter fw = new FileWriter(f);
            fw.write("{\n");
            fw.write("  \"registered\": " + mIsRegistered + ",\n");
            fw.write("  \"connected\": " + (mConnectionState == BluetoothProfile.STATE_CONNECTED) + ",\n");
            fw.write("  \"state\": \"" + stateToString(mConnectionState) + "\",\n");
            fw.write("  \"target\": \"" + (mTargetDevice != null ? mTargetDevice.getName() : "none") + "\",\n");
            fw.write("  \"address\": \"" + (mTargetDevice != null ? mTargetDevice.getAddress() : "none") + "\"\n");
            fw.write("}\n");
            fw.close();
        } catch (Exception ignored) {}
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null) {
            String action = intent.getAction();
            String cmd = intent.getStringExtra("cmd");

            if ("stop".equalsIgnoreCase(cmd)) {
                stopServiceCompletely();
                return START_NOT_STICKY;
            } else if (ACTION_LOCK.equals(action) || "lock".equalsIgnoreCase(cmd)) {
                sendWinL();
            } else if (ACTION_TYPE.equals(action) || "type".equalsIgnoreCase(cmd)) {
                String text = intent.getStringExtra("text");
                typeText(text);
            } else if ("combo".equalsIgnoreCase(cmd)) {
                String keys = intent.getStringExtra("keys");
                sendCombo(keys);
            } else if ("connect".equalsIgnoreCase(cmd)) {
                connectTarget();
            } else if ("disconnect".equalsIgnoreCase(cmd)) {
                disconnectTarget();
            } else if ("unpair".equalsIgnoreCase(cmd)) {
                unpairTarget();
            } else if ("register".equalsIgnoreCase(cmd)) {
                registerHidApp();
            } else if ("status".equalsIgnoreCase(cmd)) {
                log("STATUS: Registered=" + mIsRegistered + ", State=" + stateToString(mConnectionState) + ", Target=" + (mTargetDevice != null ? mTargetDevice.getName() : "none"));
                writeStatusFile();
            }
        }
        return START_STICKY;
    }

    public boolean isRegistered() { return mIsRegistered; }
    public int getConnectionState() { return mConnectionState; }
    public BluetoothDevice getTargetDevice() { return mTargetDevice; }

    public String stateToString(int state) {
        switch (state) {
            case BluetoothProfile.STATE_DISCONNECTED: return "DISCONNECTED";
            case BluetoothProfile.STATE_CONNECTING: return "CONNECTING...";
            case BluetoothProfile.STATE_CONNECTED: return "CONNECTED";
            case BluetoothProfile.STATE_DISCONNECTING: return "DISCONNECTING...";
            default: return "UNKNOWN(" + state + ")";
        }
    }

    public void log(String msg) {
        final String line = "[" + mTimeFormat.format(new Date()) + "] " + msg;
        Log.d(TAG, msg);
        mHandler.post(() -> {
            synchronized (mListeners) {
                for (StateListener l : mListeners) {
                    l.onLog(line);
                }
            }
        });
    }

    private void notifyStateChanged() {
        mHandler.post(() -> {
            synchronized (mListeners) {
                for (StateListener l : mListeners) {
                    l.onStateUpdated();
                }
            }
        });
    }

    @Override
    public IBinder onBind(Intent intent) {
        return mBinder;
    }

    public void stopServiceCompletely() {
        log("Stopping HidService and freeing Bluetooth resources...");
        disconnectTarget();
        if (mHidDevice != null) {
            try {
                mHidDevice.unregisterApp();
            } catch (Exception ignored) {}
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            stopForeground(STOP_FOREGROUND_REMOVE);
        } else {
            stopForeground(true);
        }
        stopSelf();
    }

    @Override
    public void onDestroy() {
        super.onDestroy();
        sInstance = null;
        if (mHidDevice != null) {
            try {
                mHidDevice.unregisterApp();
            } catch (Exception ignored) {}
            if (mAdapter != null) {
                mAdapter.closeProfileProxy(BluetoothProfile.HID_DEVICE, mHidDevice);
            }
            mHidDevice = null;
        }
        mConnectionState = BluetoothProfile.STATE_DISCONNECTED;
        writeStatusFile();
    }
}
