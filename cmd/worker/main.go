// Command worker runs the single consultancy-automation worker binary.
//
// It registers all generic handlers (internal/handlers) and pulls tasks
// from NATS. Secrets resolve from the environment at task time; see
// internal/secret and secrets/README.md.
//
// Optional: set OPS_HTTP_ADDR (e.g. 127.0.0.1:8090) to also serve the
// human-approval shim (internal/opshttp) from this same process.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/danmestas/dagnats/worker"
	"github.com/nats-io/nats.go"

	"github.com/calionauta/turvez/internal/handlers"
	"github.com/calionauta/turvez/internal/opshttp"
	"github.com/calionauta/turvez/internal/secret"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}

func run() error {
	if err := secret.InitFromEnv(); err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}
	natsURL := secret.Optional("DAGNATS_NATS_URL", "")
	if natsURL == "" {
		natsURL = secret.Optional("NATS_URL", nats.DefaultURL)
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect NATS %s: %w", natsURL, err)
	}
	defer nc.Close()

	w := worker.NewWorker(nc)
	handlers.RegisterAll(w)
	w.Start()
	defer w.Stop()
	log.Printf("worker: connected to %s, handlers registered", natsURL)

	if addr := os.Getenv("OPS_HTTP_ADDR"); addr != "" {
		s := &opshttp.Server{
			Token:      os.Getenv("OPS_APPROVAL_TOKEN"),
			DagnatsAPI: secret.Optional("DAGNATS_API_URL", "http://127.0.0.1:8080"),
		}
		if s.Token == "" {
			return fmt.Errorf("OPS_HTTP_ADDR is set but OPS_APPROVAL_TOKEN is empty")
		}
		go func() {
			log.Printf("worker: ops approvals on %s", addr)
			if err := http.ListenAndServe(addr, s.Handler()); err != nil {
				log.Printf("worker: ops http: %v", err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("worker: shutting down")
	return nil
}
