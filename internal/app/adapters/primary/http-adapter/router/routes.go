package router

import (
	"net/http"

	"github.com/rostislaved/go-clean-architecture/internal/app/adapters/primary/http-adapter/handlers"
	middlewarehelpers "github.com/rostislaved/go-clean-architecture/internal/pkg/middleware-helpers"
)

func (r *Router) AppendRoutes(config Config, handlers *handlers.Handlers) {
	r.config = config

	apiV1Subrouter := r.router.PathPrefix(apiV1Prefix).Subrouter()

	accessGuard := middlewarehelpers.HostOriginGuard(
		[]string{"localhost", "127.0.0.1", "::1", "pay.bog-best-online-games.ru"},
		[]string{"https://pay.bog-best-online-games.ru"},
	)

	routes := []Route{
		{
			Name:    "method1",
			Path:    "/method1",
			Method:  http.MethodPost,
			Handler: middlewarehelpers.And(accessGuard)(http.HandlerFunc(handlers.Get)),
		},
		{
			Name:    "openvpn-certificate",
			Path:    "/openvpn/certificates",
			Method:  http.MethodPost,
			Handler: middlewarehelpers.And(accessGuard)(http.HandlerFunc(handlers.EnsureOpenVPNClient)),
		},
		{
			Name:    "openvpn-certificate-revoke",
			Path:    "/openvpn/certificates/revoke",
			Method:  http.MethodPost,
			Handler: middlewarehelpers.And(accessGuard)(http.HandlerFunc(handlers.RevokeOpenVPNClient)),
		},
	}

	r.appendRoutesToRouter(apiV1Subrouter, routes)
}
