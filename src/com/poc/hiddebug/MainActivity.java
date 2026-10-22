package com.poc.hiddebug;

import android.app.Activity;
import android.bluetooth.BluetoothAdapter;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.view.View;
import android.widget.Button;
import android.widget.ScrollView;
import android.widget.TextView;

public class MainActivity extends Activity implements HidService.StateListener {

    private HidService mService;
    private boolean mBound = false;

    private TextView tvSummary;
    private TextView tvLogs;
    private ScrollView svLog;

    private final ServiceConnection mConn = new ServiceConnection() {
        @Override
        public void onServiceConnected(ComponentName name, IBinder binder) {
            HidService.LocalBinder b = (HidService.LocalBinder) binder;
            mService = b.getService();
            mBound = true;
            mService.addListener(MainActivity.this);
            updateSummary();
            onLog("[UI] Connected to HidService background process.");
        }

        @Override
        public void onServiceDisconnected(ComponentName name) {
            mService = null;
            mBound = false;
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);

        tvSummary = findViewById(R.id.tvSummary);
        tvLogs = findViewById(R.id.tvLogs);
        svLog = findViewById(R.id.svLog);

        findViewById(R.id.btnRegister).setOnClickListener(v -> {
            if (mService != null) mService.registerHidApp();
        });
        findViewById(R.id.btnDiscoverable).setOnClickListener(v -> makeDiscoverable());
        findViewById(R.id.btnRefresh).setOnClickListener(v -> {
            if (mService != null) mService.refreshDevices();
        });
        findViewById(R.id.btnConnect).setOnClickListener(v -> {
            if (mService != null) mService.connectTarget();
        });
        findViewById(R.id.btnDisconnect).setOnClickListener(v -> {
            if (mService != null) mService.disconnectTarget();
        });
        findViewById(R.id.btnUnpair).setOnClickListener(v -> {
            if (mService != null) mService.unpairTarget();
        });
        findViewById(R.id.btnWinL).setOnClickListener(v -> {
            if (mService != null) mService.sendWinL();
        });
        findViewById(R.id.btnTestKey).setOnClickListener(v -> {
            if (mService != null) mService.typeText("A");
        });
        findViewById(R.id.btnClearLog).setOnClickListener(v -> tvLogs.setText(""));

        checkPermissions();
        startAndBindService();
    }

    private void startAndBindService() {
        Intent intent = new Intent(this, HidService.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent);
        } else {
            startService(intent);
        }
        bindService(intent, mConn, Context.BIND_AUTO_CREATE);
    }

    private void checkPermissions() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            String[] perms = new String[] {
                "android.permission.BLUETOOTH_CONNECT",
                "android.permission.BLUETOOTH_ADVERTISE",
                "android.permission.BLUETOOTH_SCAN",
                "android.permission.POST_NOTIFICATIONS"
            };
            for (String p : perms) {
                int status = checkSelfPermission(p);
                if (status != PackageManager.PERMISSION_GRANTED) {
                    requestPermissions(perms, 100);
                    break;
                }
            }
        }
    }

    private void makeDiscoverable() {
        try {
            Intent discoverableIntent = new Intent(BluetoothAdapter.ACTION_REQUEST_DISCOVERABLE);
            discoverableIntent.putExtra(BluetoothAdapter.EXTRA_DISCOVERABLE_DURATION, 300);
            startActivity(discoverableIntent);
        } catch (Exception ignored) {}
    }

    @Override
    public void onLog(String logLine) {
        runOnUiThread(() -> {
            if (tvLogs != null) {
                tvLogs.append(logLine + "\n");
                svLog.post(() -> svLog.fullScroll(View.FOCUS_DOWN));
            }
        });
    }

    @Override
    public void onStateUpdated() {
        runOnUiThread(this::updateSummary);
    }

    private void updateSummary() {
        if (mService == null) return;
        StringBuilder sb = new StringBuilder();
        sb.append("HID Service: RUNNING (Persistent Foreground)");
        sb.append("\nApp Registered: ").append(mService.isRegistered() ? "YES (SDP OK)" : "NO");
        sb.append("\nTarget: ").append(mService.getTargetDevice() != null ? mService.getTargetDevice().getName() + " (" + mService.getTargetDevice().getAddress() + ")" : "None");
        sb.append(" [").append(mService.stateToString(mService.getConnectionState())).append("]");
        tvSummary.setText(sb.toString());
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (mBound) {
            if (mService != null) mService.removeListener(this);
            unbindService(mConn);
            mBound = false;
        }
    }
}
