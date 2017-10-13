/*************************************************************************
                                Helpers
*************************************************************************/

// Attempts to fetch the API key from the browsers cookies
function getAPIKey() {
  cookies = document.cookie.split(';');
  for (var i = 0; i < cookies.length; i++) {
      c = cookies[i];
      while (c.charAt(0) === ' ') c = c.substring(1, c.length);
      if (c.indexOf("token=") === 0) {
        return c.substring(6, c.length);
      }
  }
  return null;
}

// Sends a message to the GUI server with the correct authorization/format
function sendMessage(endpoint, data, callback, callbackErr){
  $.ajax({
    url: 'http://localhost:8080/' + endpoint,
    type: 'post',
    data: data,
    headers: {
        Authorization: 'Bearer ' + getAPIKey()
    },
    success: callback,
    error: callbackErr
  })
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
  // By default, display the general status page
  loadStatus("general");

  // Load the version and hostname data into the top bar
  sendMessage("agent/version", "", function(data, status, xhr) {
    $("#version").append(data.Major + "." + data.Minor + "." + data.Patch);
  });
  sendMessage("agent/hostname", "", function(data, status, xhr) {
    $("#hostname").append(JSON.stringify(data))
  });

  // Regularily check if agent is running
  setInterval(checkStatus, 2000);
}

// Tests the connection to the Agent and displays the appropriate response in the top bar
function checkStatus() {
  sendMessage("agent/ping", "",
  function(data, status, xhr) {
    $("#agent_status").html("Connected <br>to Agent");
    $("#agent_status").css({
      "background": 'linear-gradient(to bottom, #89c403 5%, #77a809 100%)',
      "background-color": '#89c403',
      "border": '1px solid #74b807',
      "text-shadow": '0px 1px 0px #528009',
      'left': '-150px'
    })
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

  sendMessage("agent/status/" + page, "",
  function(data, status, xhr){
      $("#" + page + "_status").html(data);
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
  sendMessage("agent/log/true", "",
  function(data, status, xhr){
    // Remove newline at the start
    if (data.substring(0, 4) == "<br>") data = data.substring(4, data.length);

    // Initially load a maximum number of lines (but allow for loading more)
    data = trimData(data);

    $("#logs").html('<div class="log_title">Agent.log' +
                    '<span class="custom-dropdown"> <select id="log_view_type">' +
                    '<option value="recent_first" selected>Most recent first</option>' +
                    '<option value="old_first">Oldest first</option>' +
                    '</select></span></div>' +
                    '<div class="log_data">' + data + ' </div>');
    $("#log_view_type").change(changeLogView);
  }, function(){
    $('#logs').html("<span class='center'>An error occurred.</span>");
  });
}

// Handler for when the log view dropdown changes
function changeLogView() {
  if ($("#log_view_type").val() == "old_first") flip = "false";
  else flip = "true";

  sendMessage("agent/log/" + flip, "",
  function(data, status, xhr){
    if (data.substring(0, 4) == "<br>") data = data.substring(4, data.length);
    data = trimData(data);

    $(".log_data").html(data);
  });
}

// Helper function which trims the next 200 lines off extraData and returns it
var extraData;
function trimData(data) {
  linesToLoad = 200;
  i = -1;

  // Find the index of the 200th occurrence of <br>
  while (linesToLoad > 0 && i < data.length) {
    i = data.indexOf("<br>", i);
    if (i < 0) break;
    linesToLoad--;
    i++;
  }

  if (i > 0) {    // if the 200th <br> exists
    extraData = data.substring(i+3, data.length);
    data = data.substring(0, i-1);

    // Add a way to load more
    data += "<br><a href='javascript:void(0)' onclick='loadMore()' class='load_more'> Load more </a>";
  }
  return data;
}

// Handler for loading more lines of the currently displayed log file
function loadMore() {
  data = $(".log_data").html();

  // Remove the load more button
  i = data.lastIndexOf("<a href=");
  data = data.substring(0, i);

  // Add the next 150 lines
  $(".log_data").html(data + trimData(extraData));
}


/*************************************************************************
                              Settings
*************************************************************************/

// Fetches the configuration file and displays in on the settings page
function loadSettings() {
  $(".page").css("display", "none");
  $("#settings").css("display", "block");

  $('#settings').html('<textarea id="settings_input"></textarea>' +
                      '<div id="submit_settings">Save</div>');
  $("#submit_settings").click(submitSettings);

  sendMessage("agent/getConfig", "",
  function(data, status, xhr){
    $('#settings_input').val(data);
  }, function(){
    $('#settings').html("<span class='center'>An error occurred.</span>");
  });
}

// Handler for the 'submit settings' button, sends the configuration file back to the server to save
function submitSettings() {
  settings = $("#settings_input").val();

  sendMessage("agent/setConfig", JSON.stringify({config: settings}),
  function(data, status, xhr) {
    resClass = "success"; symbol = "fa-check";
    resMsg = "Restart agent <br> to see changes";

    if (data != "Success") {
      console.log(data);
      resClass = "unsuccessful"; symbol = "fa-times";

      if (data.includes("permission denied")) resMsg = "Permission <br> denied";
      else resMsg = "An error <br> occurred";
    }

    $("#submit_settings").append('<i class="fa ' + symbol + ' fa-lg ' + resClass + '"></i>' +
                                  '<div class="msg">' + resMsg + '</div>');
    $("." + resClass + ", .msg").delay(3000).fadeOut("slow");
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
function loadCheckFiles() {
  $(".list").html("");

  sendMessage("checks/list/yaml", "",
  function(data, status, xhr){
    if (typeof(data) == "string") return $("#checks_description").html(data);
    $("#checks_description").html("Select a check to configure.");

    data.sort();
    data.forEach(function(item){
      if (item.substr(item.length - 5) == ".yaml" || item.substr(item.length - 13) == ".yaml.default") {
        $(".list").append('<a href="javascript:void(0)" onclick="showCheckConfig(\''
                          + item  + '\')" class="check">' +  item + '</a>');
      }
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

  // Get a list of all the currently enabled checks
  enabledChecks = [];
  sendMessage("checks/list/yaml", "",
  function(data, status, xhr){
    if (typeof(data) == "string") return;
    data.sort();
    data.forEach(function(item){
      if (item.substr(item.length - 5) == ".yaml") {
        enabledChecks.push(item.substr(0, item.length - 5));
      }
    });

    // Get a list of all the check (.py) files
    sendMessage("checks/list/py", "",
    function(data, status, xhr){
      if (typeof(data) == "string") return $("#checks_description").html(data);

      $("#checks_description").html("Select a check to add.");
      data.sort();
      data.forEach(function(item){
        // Only display checks that aren't already enabled
        if (item.substr(item.length - 3) != ".py" ||
            enabledChecks.indexOf(item.substr(0, item.length - 3)) != -1) return;

        $(".list").append('<a href="javascript:void(0)" onclick="addCheck(\'' +
                          item.substr(0, item.length - 3) + '\')" class="check">' +  item + '</a>');
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
  val = $("#checks_dropdown").val();
  $(".right").html("");

  if (val == "enabled") loadCheckFiles();
  else if (val == "add") loadNewChecks();
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

  sendMessage("checks/getConfig/" + fileName, "",
  function(data, status, xhr) {
    $(".right").html('<div id="save_check">Save</div>' +
                     '<div id="reload_check" class="inactive">Reload</div>' +
                     '<textarea id="check_input">' + data + '</textarea>');
    $('#check_input').data('file_name',  fileName);
    $('#check_input').data('check_name',  fileName.substr(0, fileName.indexOf(".")));   // remove the ending
    $("#save_check").click(saveCheckSettings);
    $("#reload_check").click(reloadCheck);
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

// Handler for the save button, sends a check configuration file to the server to be saved
function saveCheckSettings() {
  settings = $("#check_input").val();
  fileName = $('#check_input').data('file_name');

  // If the check was a default check, save this config as a new, non-default config file
  if (fileName.substr(fileName.length - 8) == ".default") {
    fileName = fileName.substr(0, fileName.length-8)
  }

  sendMessage("checks/setConfig/" + fileName, JSON.stringify({config: settings}),
  function(data, status, xhr) {
    resClass = "success"; symbol = "fa-check";
    resMsg = "Reload check <br> to see changes";
    if (data != "Success") {
      resClass = "unsuccessful"; symbol = "fa-times";
      if (data.includes("permission denied")) resMsg = "Permission <br> denied";
      else resMsg = "An error occurred: " + data;
    }

    $("." + resClass + ", .msg").remove();
    $("#save_check").append('<i class="fa ' + symbol + ' fa-lg ' + resClass + '"></i><div class="msg">' + resMsg + '</div>');
    $("." + resClass + ", .msg").delay(3000).fadeOut("slow");

    if (resClass == "success") {
      $("#reload_check").removeClass("inactive");
      $('#check_input').data('file_name', fileName);    // in case we removed the .default ending

      // Reload the list of file names, because if it was a default check it should become non-default
      loadCheckFiles();
      $("#checks_description").html("Edit the configuration file, then save and reload.");
    }
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

// Handler for the reload button, tells the server to run the check once as a test, if it's
// a success it reloads the check (also displays the tests results as a popup)
function reloadCheck() {
  $("#reload_check").addClass("inactive");
  name = $('#check_input').data('check_name');

  // Test it once with new configuration
  sendMessage("checks/run/" + name + "/once", "",
  function(data, status, xhr){
    $("#manage_checks").append("<div id='popup'>" + data["html"] + "<div class='exit'>x</div></div>");
    $(".exit").click(function() {
      $("#popup").remove();
      $(".exit").remove();
    });

    // If check test was successful, reload the check
    if (data["success"]) {
      $("#check_run_results").prepend('<div id="summary">Check reloaded: <i class="fa fa-check green"></div>');
      sendMessage("checks/reload/" + name, "",
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

// Starts the process of adding a new check by seeing if that check has an example config file
function addCheck(checkName) {
  $(".right").html("");
  // See if theres an example file for this check

  sendMessage("checks/list/yaml", "",
  function(data, status, xhr){
    exampleFile = false;
    if (typeof(data) != "string") {
      data.forEach(function(item){
        if (item == checkName + ".yaml.example") exampleFile = true;
      });
    }
    createNewConfigFile(checkName, exampleFile);
  }, function() {
    $("#checks_description").html("An error occurred.");
  });
}

// Creates a text area for the user to create a new configuration file, and if there's an
// example file for that check displays a button for seeing this example file
function createNewConfigFile(checkName, exampleFile) {
  $("#checks_description").html("Please create a new configuration file for this check below.");
  $(".right").append('<textarea id="new_config_input">Add your configuration here.</textarea>' +
                     '<div id="add_check">Add Check</div>');

  $("#add_check").click(function(){
    $("#add_check").css("pointer-events", "none");
    addNewCheck(checkName + ".yaml");
  });

  if (!exampleFile) return;
  $(".right").append('<div id="see_example">See Example File</div>');
  $("#see_example").click(function(){ displayExample(checkName); });
}

// Handler for the 'See example file' button, displays the current check's example configuration file
// as a popup
function displayExample(checkName) {
  // Fetch the example configuration file and display it as a popup
  $("#save_run_check").css("pointer-events", "none");
  sendMessage("checks/getConfig/" + checkName + ".yaml.example", "",
  function(data, status, xhr){
    $("#manage_checks").append("<div id='popup'>" +
                                  "<div id='example_title'>" + checkName + ".yaml example: </div>" +
                                  "<textarea readonly='readonly' id='example'>" + data + "</textarea>" +
                                  "<div class='exit'>x</div>" +
                                "</div>");
    $(".exit").click(function() {
      $("#popup").remove();
      $(".exit").remove();
      $("#add_check").css("pointer-events", "auto");
    });
  }, function() {
    $("#checks_description").html("An error occurred.");
    $(".right").html("");
  });
}

// Handler for the 'add check' button: saves the file, tests the check, schedules it if
// appropriate and reloads the view
function addNewCheck(name) {
  // Save the new configuration file
  settings = $("#new_config_input").val();
  sendMessage("checks/setConfig/" + name, JSON.stringify({config: settings}),
  function(data, status, xhr){
    if (data != "Success") return $("#checks_description").html("Error saving configuration: " + data);

    // Run the check once (as a test) & print the result as a popup
    name = name.substr(0, name.length-5) // remove the .yaml
    sendMessage("checks/run/" + name + "/once", "",
    function(data, status, xhr){
      $("#manage_checks").append("<div id='popup'>" + data["html"] + "<div class='exit'>x</div></div>");
      $(".exit").click(function() {
        $("#popup").remove();
        $(".exit").remove();
        $("#add_check").css("pointer-events", "auto");
      })

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

        sendMessage("checks/run/" + name, "")
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

  sendMessage("checks/running", "",
  function(data, status, xhr){
    if (data == null) {
      $("#running_checks").html("No checks ran yet.");
      return
    }

    // Count the number of instances of each check
    runningChecks = {}
    data.sort();
    data.forEach(function(name){
      if (runningChecks.hasOwnProperty(name)) runningChecks[name] += 1;
      else runningChecks[name] = 1;
    });

    $("#running_checks").html('<table id="running_checks_table">' +
                              '<tr> <th>Check Name</th>' +
                              '<th class="l_space">Number of Instances</th></tr> ' +
                              '</table>' +
                              '<div id="running_checks_info"> See Collector Status for more information.</div>');

    for (check in runningChecks) {
      $("#running_checks_table").append('<tr> <td>' + check + '</td>' +
                                        '<td class="l_space">' + runningChecks[check] + '</td></tr> ');
    }
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
  $("#flare_description").html("Your logs and configuration files will be collected and sent to DataDog Support.");
}

// Handler for the 'submit flare' button, validates the email address & then
// sends the inputted data to the server for creating a flare
function submitFlare() {
  ticket = $("#ticket_num").val();
  if (ticket == "") ticket = "0";

  email = $("#email").val();
  regex = /\S+@\S+\.\S+/;   // string - @ - string - . - string
  if ( !regex.test(email) ) {
      $("#flare_description").html("Please enter a valid email address.");
      return;
  }

  sendMessage("agent/flare", JSON.stringify({email: email, caseID: ticket}),
  function(data, status, xhr){
    $("#ticket_num").val("");
    $("#email").val("");
    $(".flare_input").css("display", "none");
    $("#flare_description").html(data);
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

  sendMessage("agent/restart", "",
  function(data, status, xhr){
    // Wait a few seconds to give the server a chance to restart
    setTimeout(function(){
      $(".loading_spinner").remove();
      $("#restart_button").css("pointer-events", "auto");

      if (data != "Success") {
        $("#general_status").css("display", "block");
        $('#general_status').html("<span class='center'>Error restarting agent: " + data + "</span>");
      } else loadStatus("general");
    }, 2000);
  }, function() {
    $(".loading_spinner").remove();
    $("#general_status").css("display", "block");
    $('#general_status').html("<span class='center'>An error occurred.</span>");
    $("#restart_button").css("pointer-events", "auto");
  });
}
