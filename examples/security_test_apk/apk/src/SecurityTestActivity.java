package com.sectest.bindersandbox;

import android.app.Activity;
import android.os.Bundle;
import android.util.Log;
import android.widget.TextView;

import java.io.BufferedReader;
import java.io.InputStreamReader;

/**
 * Runs the native binder probe binary from the app's native lib directory
 * and displays the output. The binary runs under the app's UID (normal
 * sandbox), not shell. The native lib directory has exec permission,
 * unlike the app's files directory on modern Android.
 */
public class SecurityTestActivity extends Activity {
    private static final String TAG = "BinderSecTest";
    private static final String BINARY_NAME = "libsecurity_test.so";

    private TextView outputView;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(getResources().getIdentifier("activity_main", "layout", getPackageName()));
        outputView = findViewById(getResources().getIdentifier("output", "id", getPackageName()));

        new Thread(this::runProbe).start();
    }

    private void runProbe() {
        StringBuilder sb = new StringBuilder();
        sb.append("UID: ").append(android.os.Process.myUid()).append("\n");
        sb.append("PID: ").append(android.os.Process.myPid()).append("\n\n");
        updateUI(sb.toString());

        try {
            String binaryPath = getApplicationInfo().nativeLibraryDir + "/" + BINARY_NAME;
            appendOutput(sb, "Binary: " + binaryPath + "\n");
            appendOutput(sb, "Running probe...\n\n");

            ProcessBuilder pb = new ProcessBuilder(binaryPath);
            pb.redirectErrorStream(true);
            pb.environment().put("HOME", getFilesDir().getAbsolutePath());
            Process proc = pb.start();

            BufferedReader reader = new BufferedReader(
                    new InputStreamReader(proc.getInputStream()));
            String line;
            while ((line = reader.readLine()) != null) {
                Log.i(TAG, line);
                appendOutput(sb, line + "\n");
            }

            int exitCode = proc.waitFor();
            appendOutput(sb, "\nExit code: " + exitCode + "\n");

        } catch (Exception e) {
            Log.e(TAG, "Probe failed", e);
            appendOutput(sb, "\nEXCEPTION: " + e.getMessage() + "\n");
        }
    }

    private void appendOutput(StringBuilder sb, String text) {
        sb.append(text);
        updateUI(sb.toString());
    }

    private void updateUI(String text) {
        final String t = text;
        runOnUiThread(() -> outputView.setText(t));
    }
}
