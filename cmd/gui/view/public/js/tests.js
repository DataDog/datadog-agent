
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

function testSuccess() {
  $(".page").css("display", "none");
  $("#tests").css("display", "block");

  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      req_type: "test",
      data: "test"
    }),
    headers: {
        Authorization: 'Bearer ' + getAPIKey()
    },
    success: function(data, status, xhr) {
      var ct = xhr.getResponseHeader("content-type") || "";
      if (ct.indexOf('json') != -1 ) {
        data = JSON.stringify(data);
      }
      $('#tests').html("Received response. Status: " + status +
                        "<br> Content: " + data);
    }
  });
};

// No headers
// Expected: returns error 401 (Unauthorized)
function testBadReq1() {
  $(".page").css("display", "none");
  $("#tests").css("display", "block");
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'post',
    data: JSON.stringify({
      data: "test"
    }),
    success: function(data, status, xhr) {
      $('#tests').html(data)
    }, error: function(xhr, status, err) {
      $('#tests').html(status + " " + err)
    }
  })
};

// Header present, incorrect format
// Expected: returns error 401 (Unauthorized)
function testBadReq2() {
  $(".page").css("display", "none");
  $("#tests").css("display", "block");
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
      $('#tests').html(data)
    }, error: function(xhr, status, err) {
      $('#tests').html(status + " " + err)
    }
  })
};

// Header present, correct format, incorrect key
// Expected: returns error 403 (Forbidden)
function testBadReq3() {
  $(".page").css("display", "none");
  $("#tests").css("display", "block");
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
      $('#tests').html(data)
    }, error: function(xhr, status, err) {
      $('#tests').html(status + " " + err)
    }
  })
};

// Wrong type of request
// Expected: returns error 404 (Not Found)
function testBadReq4() {
  $(".page").css("display", "none");
  $("#tests").css("display", "block");
  $.ajax({
    url: 'http://localhost:8080/req',
    type: 'get',
    data: JSON.stringify({
      data: "test"
    }),
    success: function(data, status, xhr) {
      $('#tests').html(data)
    }, error: function(xhr, status, err) {
      $('#tests').html(status + " " + err)
    }
  })
};
