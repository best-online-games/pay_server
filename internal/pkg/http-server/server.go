package http_server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

type Server struct {
	logger *slog.Logger
	server *http.Server
	config Config
}

type Config struct {
	Port              string
	StartMsg          string
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	ReadTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func New(logger *slog.Logger, config Config, handler http.Handler) *Server {
	server := &http.Server{
		Handler:           handler,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		Addr:              config.Port,
	}

	s := Server{
		logger: logger,
		server: server,
		config: config,
	}

	return &s
}

func (a *Server) Start(ctx context.Context) error {
	addr := a.server.Addr
	if addr == "" {
		addr = ":http"
	}

	startMsg := a.config.StartMsg
	if startMsg == "" {
		startMsg = "http server listening"
	}

	a.logger.Info(startMsg, "addr", addr)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-ctx.Done()

		a.logger.Info("http server shutdown initiated")

		ctx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
		defer cancel()

		err := a.server.Shutdown(ctx) //nolint:contextcheck // sic
		if err != nil {
			return err
		}

		return nil
	})

	g.Go(func() error {
		err := a.server.ListenAndServe()
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				a.logger.Info("http server stopped")
			} else {
				return err
			}
		}

		return nil
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	a.logger.Info("http server shutdown complete")

	return nil
}
