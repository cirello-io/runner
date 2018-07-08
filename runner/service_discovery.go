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
	"log"
	"net"
	"net/http"

	supervisor "cirello.io/supervisor/easy"
)

func (r *Runner) serveServiceDiscovery(ctx context.Context) error {
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
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "    ")
			r.sdMu.Lock()
			defer r.sdMu.Unlock()
			err := enc.Encode(r.dynamicServiceDiscovery)
			if err != nil {
				log.Println("cannot serve service discovery request:", err)
			}
		})

		server := &http.Server{
			Addr:    ":0",
			Handler: mux,
		}
		ctx = supervisor.WithContext(ctx, supervisor.WithLogger(log.Println))
		supervisor.Add(ctx, func(context.Context) {
			if err := server.Serve(l); err != nil {
				log.Println("service discovery server failed:", err)
			}
		})
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()
	return nil
}
