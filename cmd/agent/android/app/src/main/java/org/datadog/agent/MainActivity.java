/*
 * Copyright 2015 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

package org.datadog.agent;

import android.app.Activity;
import android.os.Bundle;
import android.os.Handler;
import android.os.Handler.Callback;

import android.util.Log;
import android.content.Intent;
import android.widget.TextView;

import java.util.Timer;
import java.util.TimerTask;


import ddandroid.Ddandroid;


public class MainActivity extends Activity {

    private TextView mTextView;
    private Timer uiTimer;
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        mTextView = (TextView) findViewById(R.id.mytextview);

        // Call Go function.
        // String greetings = Hello.greetings("Android and Gopher");
        //String greetings = "Test build";
        //mTextView.setText(greetings);
        startService(new Intent(getBaseContext(), DDService.class));

        String initialText = "Waiting to get status";
        mTextView.setText(initialText);

        // start the timer
        uiTimer = new Timer();
        uiTimer.schedule(new TimerTask() {
            @Override
            public void run() {
                TimerMethod();
            }
        }, 0, 10000);
    }

    @Override
    public void onPause() {
        super.onPause();
        uiTimer.cancel();
    }
    private void TimerMethod() {
        this.runOnUiThread(Timer_Func);
    }

    private Runnable Timer_Func = new Runnable() {
        private int count = 0;
        public void run() {
            if (count == 0) {
                mTextView.setText("First run");
            } else {
                String statusText = Ddandroid.getStatus();
                mTextView.setText(statusText);
                Log.d("ddagent", statusText);
            }
            count++;
        }
    };
}
