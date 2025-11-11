package middlewarehelpers

import (
	"net"
	"net/http"
	"strings"
)

// HostOriginGuard creates a middleware that ensures requests are served only
// when Host and Origin headers are present in the allowed lists.
func HostOriginGuard(allowedHosts []string, allowedOrigins []string) middleware {
	hostSet := make(map[string]struct{}, len(allowedHosts))
	for _, h := range allowedHosts {
		hostSet[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.TrimSpace(o)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isAllowedHost(r.Host, hostSet) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := originSet[origin]; !ok {
					http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isAllowedHost(hostPort string, allowed map[string]struct{}) bool {
	if hostPort == "" {
		return false
	}

	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}

	host = strings.ToLower(host)
	if host == "" {
		return false
	}

	if _, ok := allowed[host]; ok {
		return true
	}

	return false
}
