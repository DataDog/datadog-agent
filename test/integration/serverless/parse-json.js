'use strict'

var readline = require('readline');
var rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
  terminal: false
});

rl.on('line', function(line){
    try {
      const obj = JSON.parse(line)
      console.log(JSON.stringify(obj, null, 2))
    } catch (e) {
      console.log(line)
    }
})