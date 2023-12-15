/*************************************************************************
                                Helpers
*************************************************************************/

// Add endsWith support to browsers (IE most notably) that may not
if (!String.prototype.endsWith) {
	String.prototype.endsWith = function(search, this_len) {
		if (this_len === undefined || this_len > this.length) {
			this_len = this.length;
		}
		return this.substring(this_len - search.length, this_len) === search;
	};
}

// Attempts to fetch the API key from the browsers cookies
function getAuthToken() {
  var cookies = document.cookie.split(';');
  for (var i = 0; i < cookies.length; i++) {
      var c = cookies[i];
      while (c.charAt(0) == ' ') c = c.substring(1, c.length);
      if (c.indexOf("authToken=") == 0) {
        return c.substring(10, c.length);
      }
  }
  return null;
}

// Sends a message to the GUI server with the correct authorization/format
function sendMessage(endpoint, data, method, callback, callbackErr){
  $.ajax({
    url: window.location.href + endpoint,
    type: method,
    data: data,
    headers: {
        Authorization: 'Bearer ' + getAuthToken()
    },
    success: callback,
    error: callbackErr
  })
}

// Generates a CodeMirror text editor object and attaches it to the specific element
function attachEditor(addTo, data) {
  var codeMirror = CodeMirror(document.getElementById(addTo), {
    lineWrapping: true,
    lineNumbers: true,
    value: data,
    mode:  "yaml"
  });
  // Map tabs to spaces (yaml doesn't allow tab characters)
  codeMirror.setOption("extraKeys", {
    Tab: function(cm) {
      var spaces = Array(cm.getOption("indentUnit") + 1).join(" ");
      cm.replaceSelection(spaces);
    }
  });

  return codeMirror
}

/*************************************************************************
                                Setup
*************************************************************************/

$(document).ready(function(){
  // Add highlighting current item functionality to the nav bar
  $(".nav_item").click(function(){
    if ($(this).hasClass("multi") || $(this).hasClass("no-active")) return;
    $(".active").removeClass("active");
    $(this).addClass("active");
  });
  $(".side_menu_item").click(function(){
    $(".active").removeClass("active");
    $(this).closest(".nav_item").addClass("active");
  })

  // Set handlers for the buttons that are always present
  $("#settings_button").click(loadSettings);
  $("#flare_button").click(loadFlare);
  $("#checks_dropdown").change(checkDropdown);
  $("#submit_flare").click(submitFlare);
  $("#log_button").click(loadLog);
  $("#restart_button").click(restartAgent)

  setupHomePage()
});

function setupHomePage() {
  // Remove restart agent div
  $("#restart_status").hide()

  // By default, display the general status page
  loadStatus("general");

  // Load the version and hostname data into the top bar
  sendMessage("agent/version", "", "post", function(data, status, xhr) {
    $("#version").append(data.Major + "." + data.Minor + "." + data.Patch);
  });
  sendMessage("agent/hostname", "", "post", function(data, status, xhr) {
    $("#hostname").append(JSON.stringify(data))
  });

  // Regularly check if agent is running
  setInterval(checkStatus, 2000);
}

// Tests the connection to the Agent and displays the appropriate response in the top bar
function checkStatus() {
  if ( typeof checkStatus.uptime == 'undefined' ) {
    // It has not... perform the initialization
    checkStatus.uptime = 0;
  }
  sendMessage("agent/ping", "", "post",
  function(data, status, xhr) {
    $("#agent_status").html("Connected <br>to Agent");
    $("#agent_status").css({
      "background": 'linear-gradient(to bottom, #89c403 5%, #77a809 100%)',
      "background-color": '#89c403',
      "border": '1px solid #74b807',
      "text-shadow": '0px 1px 0px #528009',
      'left': '-150px'
    })
    last_ts = parseInt(data)
    if (checkStatus.uptime > last_ts) {
      $("#restart_status").hide()
    }
    checkStatus.uptime = last_ts
  },function() {
    $("#agent_status").html("Not connected<br> to Agent");
    $("#agent_status").css({
      "background": 'linear-gradient(to bottom, #c62d1f 5%, #f24437 100%)',
      "background-color": '#c62d1f',
      "border": '1px solid #d02718',
      "text-shadow": '0px 1px 0px #810e05',
      'left': '-180px'
    })
  });
}


/*************************************************************************
                                Status
*************************************************************************/

// Loads the general/collector status pages
function loadStatus(page) {
  $(".page").css("display", "none");
  $("#" + page + "_status").css("display", "block");

  // Clear the page and add the loading sign (this request can take a few seconds)
  $("#" + page + "_status").html('<i class="fa fa-spinner fa-pulse fa-3x fa-fw center"></i>');

  sendMessage("agent/status/" + page, "", "post",
  function(data, status, xhr){
      $("#" + page + "_status").html(DOMPurify.sanitize(data));
  },function(){
      $("#" + page + "_status").html("<span class='center'>An error occurred.</span>");
  });
}


/*************************************************************************
                                Logs
*************************************************************************/

// Fetches the agent.log log file and displays it
function loadLog(){
  $(".page").css("display", "none");
  $("#logs").css("display", "block");

  $("#logs").html('<i class="fa fa-spinner fa-pulse fa-3x fa-fw center"></i>');

  // Initially load the log with the most recent entries first
  sendMessage("agent/log/true", "", "post",
  function(data, status, xhr){
    // Remove newline at the start
    if (data.substring(0, 4) == "<br>") data = data.substring(4, data.length);

    // Initially load a maximum number of lines (but allow for loading more)
    data = trimData(data);

    $("#logs").html('<div class="log_title">Agent.log</div>' +
                    '<div class="dropdown"><select id="log_view_type">' +
                      '<option value="recent_first" selected>Most recent first</option>' +
                      '<option value="old_first">Oldest first</option>' +
                    '</select></div>' +
                    '<div class="log_data">' + DOMPurify.sanitize(data) + ' </div>');
    $("#log_view_type").change(changeLogView);
  }, function(){
    $('#logs').html("<span class='center'>An error occurred.</span>");
  });
}

// Handler for when the log view dropdown changes
function changeLogView() {
  var flip;
  if ($("#log_view_type").val() == "old_first") flip = "false";
  else flip = "true";

  sendMessage("agent/log/" + flip, "", "post",
  function(data, status, xhr){
    if (data.substring(0, 4) == "<br>") data = data.substring(4, data.length);
    data = trimData(data);

    $(".log_data").html(data);
  });
}

// Helper function which trims the next 200 lines off extraData and returns it
var extraDataGlobal;
function trimData(data) {
  var linesToLoad = 200;
  var i = -1;

  // Find the index of the 200th occurrence of <br>
  while (linesToLoad > 0 && i < data.length) {
    i = data.indexOf("<br>", i);
    if (i < 0) break;
    linesToLoad--;
    i++;
  }

  if (i > 0) {    // if the 200th <br> exists
    extraDataGlobal = data.substring(i+3, data.length);
    data = data.substring(0, i-1);

    // Add a way to load more
    data += "<br><a href='javascript:void(0)' onclick='loadMore()' class='load_more'> Load more </a>";
  }
  return data;
}

// Handler for loading more lines of the currently displayed log file
function loadMore() {
  var data = $(".log_data").html();

  // Remove the load more button
  var i = data.lastIndexOf("<a href=");
  data = data.substring(0, i);

  // Add the next 150 lines
  $(".log_data").html(DOMPurify.sanitize(data + trimData(extraDataGlobal)));
}


/*************************************************************************
                              Settings
*************************************************************************/

// Fetches the configuration file and displays in on the settings page
function loadSettings() {
  $(".page").css("display", "none");
  $("#settings").css("display", "block");

  $('#settings').html('<div id="settings_input"><div id="submit_settings">Save</div></div>');
  sendMessage("agent/getConfig", "", "post",
  function(data, status, xhr){
    var editor = attachEditor("settings_input", data);

    $("#submit_settings").click(function() { submitSettings(editor); });
  }, function(){
    $('#settings').html("<span class='center'>An error occurred.</span>");
  });
}

// Handler for the 'submit settings' button, sends the configuration file back to the server to save
function submitSettings(editor) {
  var settings = editor.getValue();

  sendMessage("agent/setConfig", JSON.stringify({config: settings}), "post",
  function(data, status, xhr) {
    $(".success, .unsuccessful, .msg").remove();
    if (data == "Success") {
      $("#submit_settings").append('<i class="fa fa-check fa-lg success"></i>' +
                                    '<div class="msg">Restart agent <br> to see changes</div>');
      $("#restart_status").show()
    } else {
      $("#submit_settings").append('<i class="fa fa-times fa-lg unsuccessful"></i>' +
                                    '<div class="msg">' +  data + '</div>');
    }
    $(".success, .unsuccessful, .msg").delay(5000).fadeOut("slow");
  });
}


/*************************************************************************
                            Manage Checks
*************************************************************************/

// Displays the 'manage checks' page and loads whatever view the dropdown currently has selected
function loadManageChecks() {
  $(".page").css("display", "none");
  $("#manage_checks").css("display", "block");

  checkDropdown();
}

// Fetches the names of all the configuration (.yaml) files and fills the list of
// checks to configure with the configurations for all currently enabled checks
function loadCheckConfigFiles() {
  $(".list").html("");

  sendMessage("checks/listConfigs", "", "post",
  function(data, status, xhr){
    if (typeof(data) == "string") return $("#checks_description").html(DOMPurify.sanitize(data));
    $("#checks_description").html("Select a check to configure.");

    data.sort();
    data.forEach(function(item){
      // filter out the example / disabled files
      if (item.endsWith(".example") ||
          item.endsWith(".disabled") ||
          item.endsWith("metrics.yaml")||
          item.endsWith("auto_conf.yaml")) return;

      item = DOMPurify.sanitize(item)
      $(".list").append('<a href="javascript:void(0)" onclick="showCheckConfig(\''
                        + item  + '\')" class="check">' +  item + '</a>');
    });

    // Add highlighting current check functionality
    $(".check").click(function(){
      $(".active_check").removeClass("active_check");
      $(this).addClass("active_check");
    })
  }, function() {
    $("#checks_description").html("An error occurred.");
  });
}

// Fetches the names of all the check (.py) files and fills the list of checks to add
// with the checks which are not already enabled
function loadNewChecks() {
  $(".list").html("");

  // Get a list of all the currently enabled checks (aka checks with a valid config file)
  var enabledChecks = [];
  sendMessage("checks/listConfigs", "", "post",
  function(data, status, xhr){
    if (typeof(data) == "string") return;
    data.sort();
    data.forEach(function(fileName){
      if (fileName.endsWith(".example") ||
          fileName.endsWith(".disabled") ||
          fileName.endsWith("metrics.yaml")||
          fileName.endsWith("auto_conf.yaml")) return;
      var checkName = fileName.substr(0, fileName.indexOf("."));
      enabledChecks.push(checkName);
    });

    // Get a list of all the check (.py) files
    sendMessage("checks/listChecks", "", "post",
    function(data, status, xhr){
      if (typeof(data) == "string") return $("#checks_description").html(DOMPurify.sanitize(data));

      $("#checks_description").html("Select a check to add.");
      data.sort();
      data.forEach(function(item){
        var checkName;

        // Remove the '.py' ending
        if (item.substr(item.length - 3) == ".py") {
            checkName = item.substr(0, item.length - 3);
        } else {
            checkName = item;
        }

        // Only display checks that aren't already enabled
        if (enabledChecks.indexOf(checkName) != -1) return;

        $(".list").append('<a href="javascript:void(0)" onclick="addCheck(\'' +
                          DOMPurify.sanitize(checkName) + '\')" class="check">' +  DOMPurify.sanitize(item) + '</a>');
      });
      // Add current item highlighting
      $(".check").click(function(){
        $(".active_check").removeClass("active_check");
        $(this).addClass("active_check");
      })
    }, function() {
      $("#checks_description").html("An error occurred.");
    });

  }, function() {
    $("#checks_description").html("An error occurred.");
  });
}

// Handler for the manage checks dropdown, changes the view according to its value
function checkDropdown() {
  var val = $("#checks_dropdown").val();
  $(".right").html("");

  if (val == "enabled") {
    loadCheckConfigFiles();
  } else if (val == "add") {
    loadNewChecks();
  }
}


//************* Edit a check configuration

// Display a currently running check's configuration file for editing and add buttons for
// saving/reloading the check
function showCheckConfig(fileName) {
  if (fileName.indexOf(".default") != -1) {
    $("#checks_description").html("Changing a default configuration file creates a new, non-default configuration file.");
  } else {
    $("#checks_description").html("Edit the configuration file, then save and reload.");
  }

  sendMessage("checks/getConfig/" + fileName, "", "post",
  function(data, status, xhr) {
    $(".right").html('<div id="check_input">' +
                       '<div id="save_check">Save</div>' +
                       '<div id="disable_check">Disable</div>' +
                     '</div>');
    $('#check_input').data('file_name', fileName);

    var editor = attachEditor("check_input", data);
    $("#save_check").click(function() { saveCheckSettings(editor); });
    $("#disable_check").click(function() { disableCheckSettings(editor); });
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

// Handler for the save button, sends a check configuration file to the server to be saved
function saveCheckSettings(editor) {
  var settings = editor.getValue();
  var fileName = $('#check_input').data('file_name');

  // If the check was a default check, save this config as a new, non-default config file
  if (fileName.substr(fileName.length - 8) == ".default") {
    fileName = fileName.substr(0, fileName.length-8)
  }

  sendMessage("checks/setConfig/" + fileName, JSON.stringify({config: settings}), "post",
  function(data, status, xhr) {
    $(".success, .unsuccessful").remove();
    if (data == "Success") {
      $("#save_check").append('<i class="fa fa-check fa-lg success"></i>');
      $(".success").delay(3000).fadeOut("slow");
      $("#checks_description").html("Restart agent to apply changes.");

      // If this was a default file, we just saved it under a new (non-default) name,
      // so we need to change the displayed name & update the associated file name
      $('#check_input').data('file_name', fileName);
      $(".active_check").html(fileName);
      $("#restart_status").show()
    } else {
      $("#save_check").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
      $(".unsuccessful").delay(3000).fadeOut("slow");
      $("#checks_description").html(DOMPurify.sanitize(data));
    }
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

function disableCheckSettings(editor) {
  var settings = editor.getValue();
  var fileName = $('#check_input').data('file_name');

  sendMessage("checks/setConfig/" + fileName, JSON.stringify({config: settings}), "delete",
  function(data, status, xhr) {
    $(".success, .unsuccessful").remove();
    if (data == "Success") {
      $("#disable_check").append('<i class="fa fa-check fa-lg success"></i>' +
                                 '<div class="msg">Restart agent <br> to make change effective</div>');
      $("#restart_status").show()
      $(".success").delay(3000).fadeOut("slow");
      $("#checks_description").html("Disable check.");
      $("#save_check").addClass("inactive");
      $("#disable_check").addClass("inactive");
      // If this was a default file, we just saved it under a new (non-default) name,
      // so we need to change the displayed name & update the associated file name
      $('#check_input').data('file_name', fileName);
      $(".active_check").html(fileName);

      // Reload the display (once the config file is saved this check is now enabled,
      // so it gets moved to the 'Edit Running Checks' section)
      loadCheckConfigFiles();
    } else {
      $("#disable_check").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
      $(".unsuccessful").delay(3000).fadeOut("slow");
      $("#checks_description").html(DOMPurify.sanitize(data));
    }
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

// Handler for the reload button, tells the server to run the check once as a test, if it's
// a success it reloads the check (also displays the tests results as a popup)
function reloadCheck() {
  var fileName = $('#check_input').data('file_name');
  var checkName = fileName.substr(0, fileName.indexOf("."))

  // Test it once with new configuration
  sendMessage("checks/run/" + checkName + "/once", "", "post",
  function(data, status, xhr){
    $("#manage_checks").append("<div class='popup'>" + DOMPurify.sanitize(data["html"]) + "<div class='exit'>x</div></div>");
    $(".exit").click(function() {
      $(".popup").remove();
      $(".exit").remove();
    });

    // If check test run was successful, reload the check
    if (data["success"]) {
      $("#check_run_results").prepend('<div id="summary">Check reloaded: <i class="fa fa-check green"></div>');
      sendMessage("checks/reload/" + checkName, "", "post",
      function(data, status, xhr)  {
        $("#summary").append('<br>Reload results: ' + data);
      });
    } else {
      $("#check_run_results").prepend('<div id="summary"> Check reloaded: <i class="fa fa-times red"></i></div>');
    }
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}


//************* Add a check

// Handler for when a used clicks on a check to add: starts the process of adding a check
// by checking if there's an example file for it, and loading the data from this file if so
function addCheck(checkToAdd) {
  // See if theres an example file for this check
  sendMessage("checks/listConfigs", "", "post",
  function(data, status, xhr){
    var exampleFile = "";
    var disabledFile = "";
    if (typeof(data) != "string") {
      data.forEach(function(fileName) {
        var checkName = fileName.substr(0, fileName.indexOf("."))
        if (fileName.substr(fileName.length - 8) == ".example" && checkToAdd == checkName) exampleFile = fileName;
        if (fileName.substr(fileName.length - 9) == ".disabled" && checkToAdd == checkName) disabledFile = fileName;
      });
    }

    // Display the text editor, filling it with the example file's data (if it exists)
    if (disabledFile != "") {
      sendMessage("checks/getConfig/" + disabledFile, "", "post",
      function(data, status, xhr){
        createNewConfigFile(checkToAdd, data);
      }, function() {
        $(".right").html("");
        $("#checks_description").html("An error occurred.");
      });
    } else if (exampleFile != "") {
      sendMessage("checks/getConfig/" + exampleFile, "", "post",
      function(data, status, xhr){
        createNewConfigFile(checkToAdd, data);
      }, function() {
        $(".right").html("");
        $("#checks_description").html("An error occurred.");
      });
    } else {
      createNewConfigFile(checkToAdd, "# Add your configuration here");
    }
  }, function() {
    $(".right").html("");
    $("#checks_description").html("An error occurred.");
  });
}

// Creates a text editor for the user to create a new configuration file, with the
// data to display in the editor passed in
function createNewConfigFile(checkName, data) {
  $("#checks_description").html("Please create a new configuration file for this check below.");
  $(".right").html('<div id="new_config_input"><div id="add_check">Add Check</div></div>');
  var editor = attachEditor("new_config_input", data);

  $("#add_check").click(function(){
    // Disable the button after it's been clicked because if it's successful it will load a popup,
    // so we don't want the user to be able to click the button again until the popup is closed
    $("#add_check").css("pointer-events", "none");

    // Save the new configuration file
    var settings = editor.getValue();
    sendMessage("checks/setConfig/" + checkName + ".d/conf.yaml", JSON.stringify({config: settings}), "post",
    function(data, status, xhr) {
      if (data != "Success") {
        $("#checks_description").html(DOMPurify.sanitize(data));
        $("#add_check").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
        $(".unsuccessful").delay(3000).fadeOut("slow");
        $("#add_check").css("pointer-events", "auto");
        return
      }

      // Run the check once (as a test) & print the result as a popup
      $("#restart_status").show()

      // Reload the display (once the config file is saved this check is now enabled,
      // so it gets moved to the 'Edit Running Checks' section)
      checkDropdown();
    }, function() {
      $("#checks_description").html("An error occurred.");
      $(".right").html("");
    });
  });
}

// Handler for the 'add check' button: saves the file, tests the check, schedules it if
// appropriate and reloads the view
function addNewCheck(editor, name) {
  // Save the new configuration file
  var settings = editor.getValue();
  sendMessage("checks/setConfig/" + name + ".d/conf.yaml", JSON.stringify({config: settings}), "post",
  function(data, status, xhr) {
    if (data != "Success") {
      $("#checks_description").html(DOMPurify.sanitize(data));
      $("#add_check").append('<i class="fa fa-times fa-lg unsuccessful"></i>');
      $(".unsuccessful").delay(3000).fadeOut("slow");
      $("#add_check").css("pointer-events", "auto");
      return
    }

    // Run the check once (as a test) & print the result as a popup
    sendMessage("checks/run/" + name + "/once", "", "post",
    function(data, status, xhr) {
      var html;
      if (typeof(data) == "string") html = data
      else html = data["html"]
      $("#manage_checks").append("<div class='popup'>" + html + "<div class='exit'>x</div></div>");
      $(".exit").click(function() {
        $(".popup").remove();
        $(".exit").remove();
        $("#add_check").css("pointer-events", "auto");
      });

      // Reload the display (once the config file is saved this check is now enabled,
      // so it gets moved to the 'Edit Running Checks' section)
      checkDropdown();

      // If check test was successful, schedule the check (if it wasn't unsuccessful)
      // scheduling it would only trigger an error because the check doesn't run)
      if (data["success"]) {
        $("#check_run_results").prepend(
          '<div id="summary">' +
            'Check enabled: <i class="fa fa-check green"></i> <br>' +
            'Check running: <i class="fa fa-check green"></i>' +
          '</div>');

        sendMessage("checks/run/" + name, "", "post")
      } else {
        $("#check_run_results").prepend(
          '<div id="summary">' +
            'Check enabled: <i class="fa fa-check green"></i> <br>' +
            'Check running: <i class="fa fa-times red"></i>' +
          '</div>');
      }
    });
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}


/*************************************************************************
                            See Running Checks
*************************************************************************/

// Display the list of currently running checks on the running checks page
function seeRunningChecks() {
  $(".page").css("display", "none");
  $("#running_checks").css("display", "block");

  sendMessage("checks/running", "", "post",
  function(data, status, xhr){
    $("#running_checks").html(DOMPurify.sanitize(data));
  }, function() {
    $("#running_checks").html("An error occurred.");
  });
}


/*************************************************************************
                                Flare
*************************************************************************/

// Display the 'send a flare' page
function loadFlare() {
  $(".page").css("display", "none");
  $("#flare, .flare_input").css("display", "block");
  $("#flare_description").html("Your logs and configuration files will be collected and sent to Datadog Support.");
}

// Handler for the 'submit flare' button, validates the email address & then
// sends the inputted data to the server for creating a flare
function submitFlare() {
  var ticket = $("#ticket_num").val();
  if (ticket == "") ticket = "0";

  var email = $("#email").val();
  var regex = /\S+@\S+\.\S+/;   // string - @ - string - . - string
  if ( !regex.test(email) ) {
      $("#flare_description").html("Please enter a valid email address.");
      return;
  }

  sendMessage("agent/flare", JSON.stringify({email: email, caseID: ticket}), "post",
  function(data, status, xhr){
    $("#ticket_num").val("");
    $("#email").val("");
    $(".flare_input").css("display", "none");
    $("#flare_description").html(DOMPurify.sanitize(data));
  }, function(){
    $('#flare_response').html("<span class='center'>An error occurred.</span>");
  });
}


/*************************************************************************
                            Restart Agent
*************************************************************************/

// Sends a request to the server to restart the agent
function restartAgent() {
  $(".page").css("display", "none");
  $(".active").removeClass("active");
  $("#main").append('<i class="fa fa-spinner fa-pulse fa-3x fa-fw center loading_spinner"></i>');

  $("#agent_status").html("Not connected<br> to Agent");
  $("#agent_status").css({
    "background": 'linear-gradient(to bottom, #c62d1f 5%, #f24437 100%)',
    "background-color": '#c62d1f',
    "border": '1px solid #d02718',
    "text-shadow": '0px 1px 0px #810e05',
    'left': '-180px'
  });

  // Disable the restart button to prevent multiple consecutive clicks
  $("#restart_button").css("pointer-events", "none");

  sendMessage("agent/restart", "", "post",
  function(data, status, xhr){
    // Wait a few seconds to give the server a chance to restart
    setTimeout(function(){
      $(".loading_spinner").remove();
      $("#restart_button").css("pointer-events", "auto");

      if (data != "Success") {
        $("#general_status").css("display", "block");
        $('#general_status').html("<span class='center'>Error restarting agent: " + DOMPurify.sanitize(data) + "</span>");
      } else loadStatus("general");
    }, 10000);
  }, function() {
    $(".loading_spinner").remove();
    $("#general_status").css("display", "block");
    $('#general_status').html("<span class='center'>An error occurred.</span>");
    $("#restart_button").css("pointer-events", "auto");
  });
}
