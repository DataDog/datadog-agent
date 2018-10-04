package org.datadog.agent;

import android.support.v7.app.AppCompatActivity;
import android.app.Service;
import android.content.Intent;
import android.os.Bundle;
import android.os.IBinder;
import android.support.annotation.Nullable;
import android.util.Log;

import ddandroid.Ddandroid;

public class DDService extends Service {
    @Nullable
    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        // Let it continue running until it is stopped.
        Log.d("testservice", "started");
        Ddandroid.androidMain(intent.getStringExtra("api_key"), intent.getStringExtra("hostname"), intent.getStringExtra("tags"));
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        Log.d("testservice", "destroyed");
        super.onDestroy();
    }
}