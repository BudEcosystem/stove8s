package daemonset

// NOTE: daemonset should be stateless across restarts
// all the state is held in memorry

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bud.studio/stove8s/internal/daemonset/resources"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func routerInit() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Timeout(time.Second))
	r.Use(middlewareServerHeader)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Mount("/oci", resources.OciResource{}.Routes())

	return r
}

func Run() {
	serverCtx, serverCtxCancel := context.WithCancel(context.Background())

	srv := &http.Server{
		Addr:         ":8008",
		WriteTimeout: 4 * time.Second,
		ReadTimeout:  4 * time.Second,
		Handler:      routerInit(),
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig

		shutdownCtx, shutdownCtxCancel := context.WithTimeout(serverCtx, 30*time.Second)
		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal("Graceful shutdown timed out, Forcing exit")
			}
		}()

		err := srv.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}

		shutdownCtxCancel()
		serverCtxCancel()
	}()

	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-serverCtx.Done()
}
