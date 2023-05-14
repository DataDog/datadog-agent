// apmTemplate defines the template which will be displayed on the APM section of the status page.
var apmTemplate = '' +
'    Status: Running<br>' +
'    Pid: <%= pid %><br>' +
'    Uptime: <%= uptime %> seconds<br>' +
'    Mem alloc: <%= memstats.Alloc %> bytes<br>' +
'    Hostname: <%= config.Hostname %><br>' +
'    Receiver: <%= config.ReceiverHost %>:<%= config.ReceiverPort %><br>' +
'    Endpoints:' +
'    <span class="stat_subdata">' +
'        <% config.Endpoints.forEach(function(e) { %>' +
'            <%= e.Host %><br />' +
'        <% }); %>' +
'    </span>' +
'    <span class="stat_subtitle">Receiver (previous minute)</span>' +
'    <span class="stat_subdata">' +
'        <% if (!Array.isArray(receiver) || receiver.length == 0) { %>' +
'            No traces received in the previous minute.<br>' +
'        <% } else { %>' +
'            <% receiver.forEach(function(ts, i) { %>' +
'                From <% if (typeof ts.Lang != "undefined") { %>' +
'                    <%= ts.Lang %> <%= ts.LangVersion %> (<%= ts.Interpreter %>), client <%= ts.TracerVersion %>' +
'                <% } else { %>' +
'                    unknown clients' +
'                <% } %>' +
'                <span class="stat_subdata">' +
'                    Traces received: <%= ts.TracesReceived %> (<%= ts.TracesBytes %> bytes)<br>' +
'                    Spans received: <%= ts.SpansReceived %>' +
'                    <% if (typeof ts.WarnString != "undefined") { %>' +
'                        <br>WARNING: <%= ts.WarnString %><br>' +
'                    <% } %>' +
'                </span>' +
'            <% }); %>' +
'        <% } %>' +
'' +
'        <% if (typeof ratebyservice != "undefined" && ratebyservice != null) { %>' +
'            <% Object.entries(ratebyservice).forEach(function(prop) { %>' +
'                <% if (prop[0] == "service:,env:") { %>' +
'                    Default priority sampling rate: <%= prop[1].toFixed(2)*100 %>%' +
'                <% } else { %>' +
'                    Priority sampling rate for \'<%= prop[0] %>\': <% prop[1].toFixed(2)*100 %>%' +
'                <% } %>' +
'                <% if (ratelimiter.TargetRate > 1.0) { %>' +
'                    <br>WARNING: Rate-limiter keep percentage: <% ratelimiter.TargetRate.toFixed(2)*100 %>%<br>' +
'                <% } %>' +
'            <% }); %>' +
'        <% } %>' +
'    </span>' +
'    <span class="stat_subtitle">Writer (previous minute)</span>' +
'    <span class="stat_subdata">' +
'        Traces: <%= trace_writer.Payloads %> payloads, <%= trace_writer.Traces %> traces, <%= trace_writer.Events %> events, <%= trace_writer.Bytes %> bytes<br>' +
'        <% if (trace_writer.Errors > 0.0) { %>WARNING: Traces API errors (1 min): <%= trace_writer.Errors %><% } %>' +
'        Stats: <%= stats_writer.Payloads %> payloads, <%= stats_writer.StatsBuckets %> stats buckets, <%= stats_writer.Bytes %> bytes<br>' +
'        <% if (stats_writer.Errors > 0.0) { %>WARNING: Stats API errors (1 min): <%= stats_writer.Errors %><% } %>' +
'    </span>';
