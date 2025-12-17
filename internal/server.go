package internal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	logger  *slog.Logger
	manager *OpenVPNManager
	server  *http.Server
	config  Config
}

func NewServer(logger *slog.Logger, config Config) *Server {
	manager := NewOpenVPNManager(logger, config.OpenVPNBaseDir, config.OpenVPNOutputDir)

	s := &Server{
		logger:  logger,
		manager: manager,
		config:  config,
	}

	router := mux.NewRouter()
	s.setupRoutes(router)

	s.server = &http.Server{
		Handler:           router,
		Addr:              config.Port,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) setupRoutes(router *mux.Router) {
	api := router.PathPrefix("/api/v1").Subrouter()

	api.HandleFunc("/openvpn/certificates", s.withMiddleware(s.handleEnsureClient)).Methods("POST")
	api.HandleFunc("/openvpn/certificates/revoke", s.withMiddleware(s.handleRevokeClient)).Methods("POST")

	router.Use(s.corsMiddleware)
	router.Use(s.recoverMiddleware)
	router.Use(s.loggingMiddleware)
}

func (s *Server) withMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return handler
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.logger.Info("http request started",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)

		next.ServeHTTP(w, r)

		s.logger.Info("http request finished",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) handleEnsureClient(w http.ResponseWriter, r *http.Request) {
	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("ensure OpenVPN client requested", "client", name)

	config, err := s.manager.EnsureClientConfig(r.Context(), name)
	if err != nil {
		s.logger.Error("ensure OpenVPN client failed", "client", name, "error", err)
		s.handleOpenVPNError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	s.logger.Info("ensure OpenVPN client success", "client", name)
	_, _ = w.Write([]byte(config))
}

func (s *Server) handleRevokeClient(w http.ResponseWriter, r *http.Request) {
	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("revoke OpenVPN client requested", "client", name)

	err = s.manager.RevokeClient(r.Context(), name)
	if err != nil {
		s.logger.Error("revoke OpenVPN client failed", "client", name, "error", err)
		s.handleOpenVPNError(w, err)
		return
	}

	s.logger.Info("revoke OpenVPN client success", "client", name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOpenVPNError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidClientName):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrClientNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrClientAlreadyRevoked):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		s.logger.Error("openvpn operation failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("http server listening", "addr", s.server.Addr)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		<-ctx.Done()

		s.logger.Info("http server shutdown initiated")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()

		return s.server.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		err := s.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		s.logger.Info("http server stopped")
		return nil
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	s.logger.Info("http server shutdown complete")
	return nil
}

func readClientName(r *http.Request) (string, error) {
	defer r.Body.Close()

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	name := strings.TrimSpace(string(raw))
	if name == "" {
		return "", errors.New("client name is required")
	}

	if strings.HasPrefix(name, "\"") && strings.HasSuffix(name, "\"") {
		if unquoted, err := strconv.Unquote(name); err == nil {
			name = unquoted
		}
	}

	return name, nil
}
