# Foldspace stream lifetime

`logs_config.foldspace.stream_lifetime` controls the native stream's interning
(dictionary/pattern) epoch lifetime. The default remains **15 minutes**. Use a
positive Go duration with units, such as `15m`, `90s` or `299.7s`:

```yaml
logs_config:
  foldspace:
    stream_lifetime: 90s
```

The environment override is `DD_LOGS_CONFIG_FOLDSPACE_STREAM_LIFETIME=90s`.
Zero, negative, unitless, malformed and overflowing durations are rejected when
building foldspace destinations; zero must not reach the native library, where
it means rotation on every open rather than the default. The setting is read at
startup, not dynamically, and remains independent of HTTP
`connection_reset_interval`. Respect the receiving Intake's stream-age limits
when increasing it.

For an X-times accelerated replay, divide this duration and the shared
`logs_config.batch_wait` by the **effective** replay speed (after interval
rounding). This approximately preserves dictionary reset cadence per corpus
second; it does not virtualize connection timers, CPU scheduling, retries,
backpressure or message timestamps. Record the actual settings and validate
complete faithful delivery before comparing bytes with a 1x reference.
