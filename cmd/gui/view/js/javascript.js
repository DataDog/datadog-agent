

function sendMessage(data, callback) {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: data,
    headers: {
        Authorization: 'Bearer ' + API_KEY
    },
    success: callback
  })
}

/***************************** Setup *****************************/

var API_KEY;
$(document).ready(function(){
  // Get the path to the datadog.yaml file
  // TODO: read datadog.yaml to get api key
  API_KEY = "test123";

  $(".nav_item").click(function(){
    $(".active").removeClass("active");
    $(this).addClass("active");
  });

  $("#status_button").click(loadStatus);
  $("#settings_button").click(loadSettings);
  $("#flare_button").click(loadFlare);
  $("#filter_button").change(filterCheckList);

  setupHomePage()
});

function setupHomePage() {
  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: "version"
  }), function(data, status, xhr) {
    $("#version").append(data.Major + "." + data.Minor + "." + data.Patch);
  });

  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: "hostname"
  }), function(data, status, xhr) {
    $("#hostname").append(JSON.stringify(data))
  });

  // Regularily check if agent is running
  setInterval(checkStatus, 2000);
}

function checkStatus() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({ req_type: "ping" }),
    headers: {
        Authorization: 'Bearer ' + API_KEY
    },
    success: function(data, status, xhr) {
      $("#agent_status").html("Agent running");
      $("#agent_status").css({
        "background": 'linear-gradient(to bottom, #89c403 5%, #77a809 100%)',
        "background-color": '#89c403',
        "border": '1px solid #74b807',
        "text-shadow": '0px 1px 0px #528009'
      })
    },
    error: function() {
      $("#agent_status").html("Agent stopped");
      $("#agent_status").css({
        "background": 'linear-gradient(to bottom, #c62d1f 5%, #f24437 100%)',
        "background-color": '#c62d1f',
        "border": '1px solid #d02718',
        "text-shadow": '0px 1px 0px #810e05'
      })
    }
  });
}

/***************************** Status *****************************/

function loadStatus(){
  $(".page").css("display", "none");
  $("#status").css("display", "block");

  // Clear the page and add the loading sign
  $("#status").html('<i class="fa fa-spinner fa-pulse fa-3x fa-fw center"></i>');

  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: "status"
  }), function(data, status, xhr){
    var ct = xhr.getResponseHeader("content-type") || "";
    if (ct.indexOf('json') != -1 ) {
      //console.log(JSON.stringify(data, null, 2));
      printStatus(data);
      return
    }

    $('#status').html("Something went wrong. Response: " + data);
  });
}

function printStatus(data) {
  $("#status").html(
    '<div id="time" class="stat"><span class="stat_title">Clocks</span></div>' +
    '<div id="s_agent_info" class="stat"><span class="stat_title">Agent Info</span></div>' +
    '<div id="config" class="stat"><span class="stat_title">Configuration</span></div>' +
    '<div id="host_info" class="stat"><span class="stat_title">Host Info</span></div>' +
    '<div id="metadata" class="stat"><span class="stat_title">Metadata</span></div>' +
    '<div id="jmx" class="stat"><span class="stat_title">JMX Status</span></div>' +
    '<div id="auto_conf" class="stat"><span class="stat_title">AutoConfig Status</span></div>' +
    '<div id="fwder" class="stat"><span class="stat_title">Forwarder Status</span></div>' +
    '<div id="agg" class="stat"><span class="stat_title">Aggregator Status (DogStatsD)</span></div>' +
    '<div id="runner" class="stat"><span class="stat_title">Runner Status</span></div>'
  );

  $('#time').append("<span class='inserted'><br> System UTC time: " + data["time"] +
                    "<br>NTP Offset: </span>" + data["ntpOffset"]);
  $('#s_agent_info').append("<span class='inserted'><br> Version " + data["version"]+
                          "<br> Check workers: " + data["runnerStats"]["Workers"] +
                          "<br> PID: " + data["pid"] +
                          "<br> Platform: " + JSON.stringify(data["platform"]) + "</span>");
  $('#config').append("<span class='inserted'><br>Conf file: " + data["conf_file"] +
                      "<br>" + JSON.stringify(data["config"]) + "</span>");
  $('#host_info').append("<span class='inserted'><br>" + JSON.stringify(data["hostinfo"]) + "</span>");
  $('#metadata').append("<span class='inserted'><br>" + JSON.stringify(data["metadata"]) + "</span>");
  $('#jmx').append("<span class='inserted'><br>" + JSON.stringify(data["JMXStatus"]) + "</span>");
  $('#auto_conf').append("<span class='inserted'><br>" + JSON.stringify(data["autoConfigStats"]) + "</span>");
  $('#fwder').append("<span class='inserted'><br>" + JSON.stringify(data["forwarderStats"]) + "</span>");
  $('#agg').append("<span class='inserted'><br>" + JSON.stringify(data["aggregatorStats"]) + "</span>");
  $('#runner').append("<span class='inserted'><br>" + JSON.stringify(data["runnerStats"]) + "</span>");
}


/***************************** Settings *****************************/

function loadSettings() {
  $(".page").css("display", "none");
  $("#settings").css("display", "block");

  $('#settings').html('<textarea id="settings_input"></textarea>' +
                      '<div id="submit_settings">Submit</div>');
  $("#submit_settings").click(submitSettings);

  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: "settings"
  }), function(data, status, xhr){
    $('#settings_input').val(data);
  });
}

function submitSettings() {
  settings = $("#settings_input").val();
  sendMessage(JSON.stringify({
    req_type: "set",
    data: "settings",
    payload: settings
  }), function(data, status, xhr){
    if (data == "Success") {
      $("#submit_settings").append('<i class="fa fa-check fa-lg success"></i>');
      $(".success").delay(3000).fadeOut("slow");
    } else {
      $("#submit_settings").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
      $(".unsuccessful").delay(3000).fadeOut("slow");
      console.log(data);
    }
  });
}

/***************************** Checks *****************************/

// only load the config files once
var loaded = false;
function loadChecks() {
  $(".page").css("display", "none");
  $("#checks_settings").css("display", "block");

  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: "conf_list"
  }), function(data, status, xhr){

    if (loaded) return;
    data.forEach(function(item){
      if (item.substr(item.length - 4) == "yaml" || item.substr(item.length - 7) == "default") {
        $("#checks_list").append('<a href="javascript:void(0)" onclick="showCheck(\'' + item + '\')" class="check enabled">' +  item + '</a>');
      } else {
        $("#checks_list").append('<a href="javascript:void(0)" onclick="showCheck(\'' + item + '\')" class="check disabled">' +  item + '</a>');
      }
    });
    loaded = true;
  });
}

function showCheck(name) {
    sendMessage(JSON.stringify({
      req_type: "check",
      data: "get_yaml",
      payload: name
    }), function(data, status, xhr){
      $("#checks_interface").html('<div id="check_title"> Editing: ' + name + '</div>' +
                                  '<div id="submit_check">Submit</div>' +
                                  '<textarea id="check_input">' + data + '</textarea>');
      $("#submit_check").click(submitCheckSettings);
    });
}

/* MAYBE: Use this toggle to allow user to enable/disable the check

$(" ").html('<div id="check_switch" class="onoffswitch">' +
                '<input type="checkbox" name="onoffswitch" class="onoffswitch-checkbox" id="myonoffswitch" checked>' +
                '<label class="onoffswitch-label" for="myonoffswitch">' +
                    '<span class="onoffswitch-inner"></span>' +
                    '<span class="onoffswitch-switch"></span>' +
                '</label>' +
              '</div>')

$("#check_switch").click( );

*/


function submitCheckSettings() {
  settings = $("#check_input").val();
  name = $("#check_title").html().slice(10);
  sendMessage(JSON.stringify({
    req_type: "check",
    data: "set_yaml",
    payload: name + " " + settings
  }), function(data, status, xhr){
    if (data == "Success") {
      $("#submit_check").append('<i class="fa fa-check fa-lg success"></i>');
      $(".success").delay(3000).fadeOut("slow");
    } else {
      $("#submit_check").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
      $(".unsuccessful").delay(3000).fadeOut("slow");
      console.log(data);
    }
  });
}

function filterCheckList() {
  val = $("#filter_button").val();
  switch (val) {
    case "all":
      $(".enabled, .disabled").css("display", "inline-block");
      break;
    case "enabled":
      $(".disabled").css("display", "none");
      $(".enabled").css("display", "inline-block");
      break;
    case "disabled":
      $(".enabled").css("display", "none");
      $(".disabled").css("display", "inline-block");
      break;
  }
}

function loadAddCheck() {
  $(".page").css("display", "none");
  $("#flare").css("display", "block");
}

/***************************** Flare *****************************/

function loadFlare() {
  $(".page").css("display", "none");
  $("#flare").css("display", "block");
}
