package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	openvpn_domain "github.com/rostislaved/go-clean-architecture/internal/app/domain/openvpn"
)

func (h Handlers) EnsureOpenVPNClient(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.Logger.Info("ensure OpenVPN client requested", "client", name)

	config, err := h.service.EnsureOpenVPNClient(r.Context(), name)
	if err != nil {
		h.Logger.Error("ensure OpenVPN client failed", "client", name, "error", err)
		h.handleOpenVPNError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	h.Logger.Info("ensure OpenVPN client success", "client", name)
	_, _ = w.Write([]byte(config))
}

func (h Handlers) RevokeOpenVPNClient(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.Logger.Info("revoke OpenVPN client requested", "client", name)

	err = h.service.RevokeOpenVPNClient(r.Context(), name)
	if err != nil {
		h.Logger.Error("revoke OpenVPN client failed", "client", name, "error", err)
		h.handleOpenVPNError(w, err)
		return
	}

	h.Logger.Info("revoke OpenVPN client success", "client", name)
	w.WriteHeader(http.StatusNoContent)
}

func (h Handlers) handleOpenVPNError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, openvpn_domain.ErrInvalidClientName):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, openvpn_domain.ErrClientNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, openvpn_domain.ErrClientAlreadyRevoked):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		h.Logger.Error("openvpn operation failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
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
		unquoted, err := strconv.Unquote(name)
		if err == nil {
			name = unquoted
		}
	}

	return name, nil
}

// setCORSHeaders ensures responses carry permissive CORS headers even if middleware is bypassed.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
