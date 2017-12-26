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
			err := enc.Encode(r.serviceDiscovery)
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
