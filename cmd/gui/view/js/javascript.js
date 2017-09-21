
var API_KEY;

$(document).ready(function(){
  // Get the path to the datadog.yaml file
  // TODO: read datadog.yaml to get api key
  API_KEY = "test123";

  $(".nav_item").click(function(){
    $(".active").removeClass("active");
    $(this).addClass("active");
  })

});

/*
function readTextFile(file) {
    var rawFile = new XMLHttpRequest();
    rawFile.open("GET", file, false);
    rawFile.onreadystatechange = function ()
    {
        if(rawFile.readyState === 4)
        {
            if(rawFile.status === 200 || rawFile.status == 0)
            {
                var allText = rawFile.responseText;
                alert(allText);
            }
        }
    }
    rawFile.send(null);
}
*/

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
    }, error: function(xhr, status, err) {
      $('#response').html(status + " " + err)
    }
  })
}

function set(m) {
  sendMessage(JSON.stringify({
    req_type: "set",
    data: m
  }));
}

function fetch(m) {
  sendMessage(JSON.stringify({
    req_type: "fetch",
    data: m
  }));
}

/**************************** Tests ***************************/

function testSuccess() {
  sendMessage(JSON.stringify({
    req_type: "test",
    data: "test"
  }));
};

// No headers
// Expected: returns error 401 (Unauthorized)
function testBadReq_1() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      data: "test"
    }),
    success: function(data, status, xhr) {
      $('#response').html(data)
    }, error: function(xhr, status, err) {
      $('#response').html(status + " " + err)
    }
  })
};

// Header present, incorrect format
// Expected: returns error 401 (Unauthorized)
function testBadReq_2() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      data: "test"
    }),
    headers: {
        Authorization: 'test123'
    },
    success: function(data, status, xhr) {
      $('#response').html(data)
    }, error: function(xhr, status, err) {
      $('#response').html(status + " " + err)
    }
  })
};

// Header present, correct format, incorrect key
// Expected: returns error 403 (Forbidden)
function testBadReq_3() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      data: "test"
    }),
    headers: {
        Authorization: 'Bearer 123'
    },
    success: function(data, status, xhr) {
      $('#response').html(data)
    }, error: function(xhr, status, err) {
      $('#response').html(status + " " + err)
    }
  })
};

// Wrong type of request
// Expected: returns error 404 (Not Found)
function testBadReq_4() {
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'get',
    data: JSON.stringify({
      data: "test"
    }),
    success: function(data, status, xhr) {
      $('#response').html(data)
    }, error: function(xhr, status, err) {
      $('#response').html(status + " " + err)
    }
  })
};
