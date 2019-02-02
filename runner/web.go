// Copyright 2017 github.com/ucirello
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runner

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	oversight "cirello.io/oversight/easy"
	terminal "github.com/buildkite/terminal-to-html"
	"github.com/gorilla/websocket"
)

func (r *Runner) subscribeLogFwd() <-chan LogMessage {
	r.logsMu.Lock()
	stream := make(chan LogMessage, websocketLogForwarderBufferSize)
	r.logSubscribers = append(r.logSubscribers, stream)
	r.logsMu.Unlock()
	return stream
}

func (r *Runner) unsubscribeLogFwd(stream <-chan LogMessage) {
	r.logsMu.Lock()
	defer r.logsMu.Unlock()
	for i := 0; i < len(r.logSubscribers); i++ {
		if r.logSubscribers[i] == stream {
			r.logSubscribers = append(r.logSubscribers[:i], r.logSubscribers[i+1:]...)
			return
		}
	}
}

func (r *Runner) serveWeb(ctx context.Context) error {
	addr := r.ServiceDiscoveryAddr
	if addr == "" {
		return nil
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Println("starting service discovery on", l.Addr())
	r.ServiceDiscoveryAddr = l.Addr().String()

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			u := url.URL{Scheme: "ws", Host: req.Host, Path: "/logs"}
			query := u.Query()
			query.Set("model", "html")
			filter := req.URL.Query().Get("filter")
			if filter != "" {
				query.Set("filter", filter)
			}
			u.RawQuery = query.Encode()
			logsPage.Execute(w, struct {
				URL    string
				Filter string
			}{u.String(), filter})
		})
		mux.HandleFunc("/discovery", func(w http.ResponseWriter, _ *http.Request) {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "    ")
			r.sdMu.Lock()
			defer r.sdMu.Unlock()
			err := enc.Encode(r.dynamicServiceDiscovery)
			if err != nil {
				log.Println("cannot serve service discovery request:", err)
			}
		})
		mux.HandleFunc("/logs", func(w http.ResponseWriter, req *http.Request) {
			filter := req.URL.Query().Get("filter")
			mode := req.URL.Query().Get("mode")
			stream := r.subscribeLogFwd()
			defer r.unsubscribeLogFwd(stream)
			upgrader := websocket.Upgrader{}
			c, err := upgrader.Upgrade(w, req, nil)
			if err != nil {
				log.Print("upgrade:", err)
				return
			}
			defer c.Close()
			for msg := range stream {
				if filter != "" && !strings.HasPrefix(msg.Name, filter) {
					continue
				}
				if mode == "html" {
					msg.Line = string(terminal.Render([]byte(msg.Line)))
				}
				b, err := json.Marshal(msg)
				if err != nil {
					log.Println("encode:", err)
					break
				}
				if err = c.WriteMessage(websocket.TextMessage, b); err != nil {
					log.Println("write:", err)
					break
				}
			}
		})

		server := &http.Server{
			Addr:    ":0",
			Handler: mux,
		}
		ctx = oversight.WithContext(ctx, oversight.WithLogger(log.New(os.Stderr, "", log.LstdFlags)))
		oversight.Add(ctx, func(context.Context) error {
			if err := server.Serve(l); err != nil {
				log.Println("service discovery server failed:", err)
			}
			return err
		})
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()
	return nil
}

var logsPage = template.Must(template.New("").Parse(`<html>
<head>
<style>
* {
	margin: 0;
	padding: 0;
}
#controlBar{
	position:fixed;
	top: 0px;
	width:100%;
	background: white;
	color: black;
	height: 25px;
	border-bottom: #c0c0c0 1pt solid;
	padding-top: 5px;
	padding-left: 5px;
}
#output{
	margin-top: 36px;
	font-family: monospace;
	white-space: pre;
	padding-left: 5px;
}
</style>
</head>
<body>
<div id="controlBar">
	<form>
		<label><input type="checkbox" id="autoScroll" checked> automatic scroll to bottom</label>
		|
		<label><input type="text" id="filter" name="filter" checked placeholder="filter by process type" value="{{.Filter}}"></label>
		<input type=submit style="display: none">
	</form>
</div>
<div id="output"></div>
<script>
var print = function(message) {
	var d = document.createElement("div");
	d.innerHTML = message;
	document.getElementById("output").appendChild(d);
};
function dial(){
	var ws = new WebSocket("{{.URL}}");
	ws.onclose = function(evt) {
		setTimeout(function(){
			print("reconnecting...")
			dial()
		}, 1000);
	}
	ws.onmessage = function(evt) {
		var msg = JSON.parse(evt.data);
		print(msg.paddedName+": "+msg.line, msg.name)
		if (document.getElementById("autoScroll").checked){
			window.scrollTo(0,document.body.scrollHeight);
		}
	}
	ws.onerror = function(evt) {
		print("ERROR: " + evt.data, "error");
	}
}
window.addEventListener("load", function(evt) {
	dial()
	return false;
});
</script>
</body>
</html>`))
