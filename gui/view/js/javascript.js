var API_KEY;

$(document).ready(function(){
  // TODO: read datadog.yaml to get api key
  API_KEY = "test123";

});

/***********************************/

function sendMessage(data) {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: data,
    headers: {
        Authorization: 'Bearer ' + API_KEY
    },
    success: function(data, status, xhr) {
      $('#response').html(data)
    }
  })
}

function sendCommand(command) {
  sendMessage(JSON.stringify({
    req_type: "command",
    data: command
  }));
}

/***********************************/

function testSuccess() {
  sendMessage(JSON.stringify({
    req_type: "command",
    data: "a"
  }));
};

// no headers
function testBadReq_1() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      req_type: "command",
      data: "b"
    }),
    success: function(data, status, xhr) {
      $('#response').html(data)
    }
  })
};

// header present, incorrect key
function testBadReq_2() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      req_type: "command",
      data: "c"
    }),
    headers: {
        Authorization: 'incorrect'
    },
    success: function(data, status, xhr) {
      $('#response').html(data)
    }
  })
};

// wrong type of request
function testBadReq_3() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'get',
    data: JSON.stringify({
      req_type: "command",
      data: "d"
    }),
    success: function(data, status, xhr) {
      $('#response').html(data)
    }
  })
};
