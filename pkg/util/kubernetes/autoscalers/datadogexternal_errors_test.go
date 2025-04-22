// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var rateLimitBodyContent = `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<title>Too many requests</title>
	<style>
		/* 960 stuff: */
		/* reset */
		html,body,div,span,applet,object,iframe,h1,h2,h3,h4,h5,h6,p,blockquote,pre,a,abbr,acronym,address,big,cite,code,del,dfn,em,font,img,ins,kbd,q,s,samp,small,strike,strong,sub,sup,tt,var,b,u,i,center,dl,dt,dd,ol,ul,li,fieldset,form,label,legend,table,caption,tbody,tfoot,thead,tr,th,td{margin:0;padding:0;border:0;outline:0;font-size:100%;vertical-align:baseline;background:transparent}body{line-height:1}ol,ul{list-style:none}blockquote,q{quotes:none}blockquote:before,blockquote:after,q:before,q:after{content:'';content:none}:focus{outline:0}ins{text-decoration:none}del{text-decoration:line-through}table{border-collapse:collapse;border-spacing:0}
		/* text */
		body{font:13px/1.5 'NotoSans','Lucida Grande','Lucida Sans Unicode',sans-serif}a:focus{outline:1px dotted}hr{border:0 #ccc solid;border-top-width:1px;clear:both;height:0}h1{font-size:25px}h2{font-size:20px}h3{font-size:16px}h4{font-size:14px}h5{font-size:13px}h6{font-size:13px}ol{list-style:decimal}ul{list-style:disc}li{margin-left:30px}p,dl,hr,h1,h2,h3,h4,h5,h6,ol,ul,pre,table,address,fieldset{margin:0 0 16px}
		.clear{clear:both;display:block;overflow:hidden;visibility:hidden;width:0;height:0}
		.clearfix:after{clear:both;content:' ';display:block;font-size:0;line-height:0;visibility:hidden;width:0;height:0}
		* html .clearfix,*:first-child+html .clearfix{zoom:1}
		/* end 960 */
		body {
			color:#333;
			background:#fff;
			min-width:980px;
		}
		a {
			color:#39c;
		}
		a:hover {
			color:#1966aa;
		}
		h1 {
			padding:3px 60px;
			color:#fff;
			background:#443e4a;
		}
		h1 img {
			vertical-align:baseline;
			margin:0 10px -8px 0;
			height:32px;
		}
		h2 {
			margin:0 0 20px;
			padding:30px 0 0;
			font-size:32px;
			line-height:36px;
		}
		.content {
			padding-left:20em;
			margin:4em auto;
			font-size:16px;
			max-width:50em;
		}
		.nocss {
			display:none;
		}
		ul {
			margin:0;
			padding:0 0 0 2em;
		}
		li {
			margin:0;
			padding:0;
		}
		.hide {
			display:none;
		}
		#color-dot {
			border-radius:99px;
			display:inline-block;
			width:10px;
			height:10px;
			margin-right:5px;
		}
		.critical {
			background-color:#e74c3c;
		}
		.major {
			background-color:#e67e22;
		}
		.minor {
			background-color:#f1c40f;
		}
		.none {
			background-color:#2ecc71;
		}
	</style>
</head>
<body>
	<h1><a href="/"><img src="/static/images/datadog-horizontal-logo.svg" alt="Datadog" /><span class="nocss">Datadog</span></a></h1>
	<div class="content">
		<img src="/static/images/datadog-error-icon.svg" alt="Sad Bits" style="float:left;margin:0 0 0 -20em;height:295px" />
		<h2>429 - Too many requests!</h2>
		<ul>
			<li id='live_status' class='hide'>Datadog status:
				<span class="meta">
					<a href="http://status.datadoghq.com" target="_blank">
						<span id="color-dot"></span>
						<span id="status-description"></span>
					</a>
				</span>
			</li>
			<li id='generic_status'><a href="http://status.datadoghq.com">Check the Datadog Status Page &rarr;</a></li>
			<li>Follow <a href="http://twitter.com/datadogops">@datadogops</a> for status updates</li>
			<li>Email us at <a href='mailto:support@datadoghq.com'>support@datadoghq.com</a></li>
			<li><a href="javascript:void(0)" onclick="location.reload()" style="text-decoration:underline">Try Datadog again &rarr;</a>
		</ul>
		<div class="maint">
			<h2 id='sched_maint' class='hide'>Scheduled Maintenance</h2>
			<div id="maint-description"></div>
			<p id='statuspage_link' class='hide'>See the <a href='http://status.datadoghq.com'>Datadog Status Page</a> for more info.</p>
		</div>
	</div>

	<script src="https://cdn.statuspage.io/se-v2.js" type="text/javascript"></script>
	<script>
		function htmlEscape(str) {
			return String(str).replace(/&/g, '&amp;')
			                  .replace(/"/g, '&quot;')
			                  .replace(/'/g, '&#39;')
			                  .replace(/</g, '&lt;')
			                  .replace(/>/g, '&gt;');
		}

		if (typeof StatusPage !== 'undefined') {
			var sp = new StatusPage.page({ page: '1k6wzpspjf99' });
			sp.status({
				success: function(data) {
					status_indicator = String(data.status.indicator);
					if (status_indicator && status_indicator.toLowerCase() !== 'none') {
						document.getElementById('color-dot').className = htmlEscape(status_indicator);
						document.getElementById('status-description').innerHTML = htmlEscape(data.status.description);
						document.getElementById('live_status').className = '';
						document.getElementById('generic_status').className = 'hide';
					}
				}
			});
			sp.scheduled_maintenances({
				filter: 'active',
				success: function(data) {
					if (
						data &&
						data.scheduled_maintenances &&
						data.scheduled_maintenances.length > 0 &&
						data.scheduled_maintenances[0].incident_updates &&
						data.scheduled_maintenances[0].incident_updates.length > 0
					) {
						document.getElementById('sched_maint').className = '';
						document.getElementById('statuspage_link').className = '';
						var desc = document.getElementById('maint-description');
						incident_text =
							'<p><b>Scheduled Start:</b> ' +
							htmlEscape(data.scheduled_maintenances[0].scheduled_for);
						incident_text +=
							'</p><p><b>Scheduled End:</b> ' +
							htmlEscape(data.scheduled_maintenances[0].scheduled_until);
						incident_text += '</p>';
						data.scheduled_maintenances[0].incident_updates.forEach(function(incident) {
							incident_text += '<p>';
							incident_text += htmlEscape(incident.body);
							incident_text += '</p>';
						});

						desc.innerHTML = String(incident_text).replace(/\n/g, '<br />');
					}
				}
			});
		}

		setTimeout(function() {
			location.reload()
		}, 60 * 1000);
	</script>

</body>
</html>`

func TestNewAPIError(t *testing.T) {
	tests := []struct {
		name        string
		inputError  error
		expectedErr *APIError
	}{
		{
			name:       "Error that does not match any known",
			inputError: errors.New("some unknown error"),
			expectedErr: &APIError{
				Code: UnknownAPIError,
				Err:  errors.New("some unknown error"),
			},
		},
		{
			name:       "Datadog API error json field set",
			inputError: errors.New("API returned error: Invalid query input:\nit is unsupported to use default mode for moving_rollup inside aggregation function moving_rollup"),
			expectedErr: &APIError{
				Code: DatadogAPIError,
				Err:  errors.New("Invalid query input: it is unsupported to use default mode for moving_rollup inside aggregation function moving_rollup"),
			},
		},
		{
			name:       "HTTP Rate limit exceeded error",
			inputError: errors.New("API error 429 Too Many Requests: " + rateLimitBodyContent),
			expectedErr: &APIError{
				Code: RateLimitExceededAPIError,
				Err:  errors.New("429 Too Many Requests"),
			},
		},
		{
			name:       "Unprocessable entity error",
			inputError: errors.New(`API error 422 Unprocessable Entity: {"errors":["Unprocessable Content"]}`),
			expectedErr: &APIError{
				Code: UnprocessableEntityAPIError,
				Err:  errors.New("422 Unprocessable Entity"),
			},
		},
		{
			name:       "Other HTTP status code error",
			inputError: errors.New("API error 500 Internal Server Error: "),
			expectedErr: &APIError{
				Code: OtherHTTPStatusCodeAPIError,
				Err:  errors.New("500 Internal Server Error"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiErr := NewAPIError(test.inputError)
			assert.Equal(t, test.expectedErr.Code, apiErr.Code)
			assert.Equal(t, test.expectedErr.Err.Error(), apiErr.Err.Error())
		})
	}
}
