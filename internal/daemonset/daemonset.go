package daemonset

// NOTE: daemonset should be stateless across restarts

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bud.studio/stove8s/internal/daemonset/resources/oci"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Config struct {
	Host string `toml:"host"`
	Port uint   `toml:"port"`
}

func routerInit() (*chi.Mux, error) {
	r := chi.NewRouter()

	r.Use(middleware.Timeout(time.Second))
	r.Use(middlewareServerHeader)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	ociHandler, err := oci.OciResource{}.Init()
	if err != nil {
		return nil, err
	}
	r.Mount("/oci", ociHandler)

	return r, nil
}

func configInit() *Config {
	config := Config{
		Host: "::",
		Port: 8008,
	}

	flag.StringVar(&config.Host, "host", config.Host, "Bind host")
	flag.UintVar(&config.Port, "port", config.Port, "Bind port")
	flag.Parse()

	return &config
}

func Run() {
	config := configInit()
	serverCtx, serverCtxCancel := context.WithCancel(context.Background())

	router, err := routerInit()
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:         net.JoinHostPort(config.Host, fmt.Sprint(config.Port)),
		WriteTimeout: 4 * time.Second,
		ReadTimeout:  4 * time.Second,
		Handler:      router,
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

	err = srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-serverCtx.Done()
}
