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
	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, err := h.service.EnsureOpenVPNClient(r.Context(), name)
	if err != nil {
		h.handleOpenVPNError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(config))
}

func (h Handlers) RevokeOpenVPNClient(w http.ResponseWriter, r *http.Request) {
	name, err := readClientName(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.service.RevokeOpenVPNClient(r.Context(), name)
	if err != nil {
		h.handleOpenVPNError(w, err)
		return
	}

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
