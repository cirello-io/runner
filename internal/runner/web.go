// Copyright 2024 github.com/ucirello, cirello.io, U. Cirello
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
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	terminal "github.com/buildkite/terminal-to-html/v3"
)

const sseLogForwarderBufferSize = 102400

func (r *Runner) subscribeLogFwd() chan LogMessage {
	stream := make(chan LogMessage, sseLogForwarderBufferSize)
	r.logsMu.Lock()
	r.logSubscribers = append(r.logSubscribers, stream)
	r.logsMu.Unlock()
	return stream
}

func (r *Runner) unsubscribeLogFwd(stream chan LogMessage) {
	r.logsMu.Lock()
	defer r.logsMu.Unlock()
	r.logSubscribers = slices.DeleteFunc(r.logSubscribers, func(i chan LogMessage) bool {
		return i == stream
	})
}

func (r *Runner) forwardLogs() {
	go func() {
		for msg := range r.logs {
			r.logsMu.RLock()
			for _, subscriber := range r.logSubscribers {
				select {
				case subscriber <- msg:
				default:
				}
			}
			r.logsMu.RUnlock()
		}
	}()
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		sseURL := url.URL{Scheme: "http", Host: req.Host, Path: "/logs"}
		query := sseURL.Query()
		query.Set("model", "html")
		filter := req.URL.Query().Get("filter")
		if filter != "" {
			query.Set("filter", filter)
		}
		sseURL.RawQuery = query.Encode()
		logsPage.Execute(w, struct {
			URL    string
			Filter string
		}{sseURL.String(), filter})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, _ *http.Request) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "    ")
		r.servicesMu.Lock()
		defer r.servicesMu.Unlock()
		err := enc.Encode(r.serviceStates)
		if err != nil {
			log.Println("cannot serve service states request:", err)
		}
	})
	mux.HandleFunc("/logs", func(w http.ResponseWriter, req *http.Request) {
		filter := req.URL.Query().Get("filter")
		mode := req.URL.Query().Get("mode")
		stream := r.subscribeLogFwd()
		defer r.unsubscribeLogFwd(stream)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		for {
			select {
			case msg := <-stream:
				if filter != "" && !strings.Contains(msg.Name, filter) && !strings.Contains(msg.Line, filter) {
					continue
				}
				if mode == "html" {
					msg.Line = string(terminal.Render([]byte(msg.Line)))
				}
				b, err := json.Marshal(msg)
				if err != nil {
					log.Println("encode:", err)
					return
				}
				_, err = w.Write([]byte("data: " + string(b) + "\n\n"))
				if err != nil {
					log.Println("write:", err)
					return
				}
				w.(http.Flusher).Flush()
			case <-req.Context().Done():
				return
			}
		}
	})
	server := &http.Server{
		Addr:    ":0",
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()
	go func() {
		if err := server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("service discovery server failed:", err)
		}
	}()
	return nil
}

var (
	//go:embed logs.tpl
	logsPageTPL string
	logsPage    = template.Must(template.New("").Parse(logsPageTPL))
)
