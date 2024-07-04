<html>
<head>
<style>
* {
	margin: 0;
	padding: 0;
}
#controlBar {
	background: white;
	border-bottom: #c0c0c0 1pt solid;
	color: black;
	min-height: 25px;
	height: auto;
	padding-left: 5px;
	padding-top: 5px;
	position:fixed;
	top: 0px;
	width: 100%;
}
#output {
	font-family: monospace;
	margin-top: 36px;
	padding-bottom: 10px;
	padding-left: 5px;
	white-space: pre;
}
#status {
	font-size: 13px;
}
IMG.badges {
	height: 20px;
	vertical-align: bottom;
}
</style>
</head>
<body>
<div id="controlBar">
	<form>
		<div>
			<label><input type="checkbox" id="autoScroll" checked> automatic scroll to bottom</label>
			|
			<label><input type="text" id="filter" name="filter" checked placeholder="filter" value="{{.Filter}}"></label>
			<input type=submit style="display: none">
			|
			<label>processes: <span id="status"><em>loading...</em></span></label>
		</div>
		<div><pre><span id="build_errors"></span></pre></div>
	</form>
</div>
<div id="output"></div>
<script>
var print = function(message) {
	var d = document.createElement("div");
	d.innerHTML = message;
	document.getElementById("output").appendChild(d);
};
function trimOutput(){
	const maxBufferSize = 262144
	if (document.getElementById("output").innerText.length > maxBufferSize) {
		document.getElementById("output").innerText = document.getElementById("output").innerText.substr(-maxBufferSize)
	}
}
function dial(){
	var es = new EventSource("{{.URL}}");
	es.onopen = function(evt) {
		print("connected...");
	};
	es.onmessage = function(evt) {
		var msg = JSON.parse(evt.data);
		print(msg.paddedName + ": " + msg.line);
		if (document.getElementById("autoScroll").checked){
			window.scrollTo(0, document.body.scrollHeight);
		}
	};
	es.onerror = function(evt) {
		print("ERROR: " + evt.data, "error");
		es.close();
		setTimeout(dial, 1000);
	};
}
var lastErr = ""
function updateStatus(){
	var xhr = new XMLHttpRequest();
	xhr.open('GET', '/discovery');
	xhr.onload = function() {
		if (xhr.status != 200) {
			console.log('Request failed.  Returned status of ' + xhr.status);
			return
		}
		var svcs = JSON.parse(xhr.responseText);
		if (svcs.length == 0) {
			return
		}
		var svc = ''
		var errors = ''
		for (i in svcs) {
			if (i.indexOf('BUILD_') === 0) {
				var name = i.substring(6)
				if (svcs[i] == "done") {
					svc += '<img class="badges" src="https://img.shields.io/badge/'+name+'-done-green.svg"/> '
				} else if (svcs[i] == "errored") {
					svc += '<img class="badges" src="https://img.shields.io/badge/'+name+'-errored-red.svg"/> '
				} else if (svcs[i] == "building") {
					svc += '<img class="badges" src="https://img.shields.io/badge/'+name+'-building-blue.svg"/> '
				} else {
					svc += '<img class="badges" src="https://img.shields.io/badge/'+name+'-unknown-lightgrey.svg"/> '
				}
			}
			if (i.indexOf('ERROR_') === 0) {
				var name = i.substring(12)
				errors += "\n"+name+"\n"+svcs[i]+"\n<hr/>"
			}
		}
		document.getElementById('status').innerHTML=svc
		if (errors !== lastErr) {
			lastErr = errors
			document.getElementById('build_errors').innerHTML=errors
		}
	};
	xhr.send();
}
window.addEventListener("load", function(evt) {
	dial()
	setInterval(updateStatus, 1000)
	setInterval(trimOutput, 1000)
	return false;
});
</script>
</body>
</html>
