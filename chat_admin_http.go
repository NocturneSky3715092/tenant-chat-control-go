package tenantchat

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type AdminHandler struct {
	service *TenantService
}

func NewAdminHandler(service *TenantService) http.Handler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(parts) == 2 && parts[0] == "tenants" && request.Method == http.MethodPost {
		account, err := h.service.Onboard(request.Context(), parts[1])
		h.write(response, account, err, http.StatusCreated)
		return
	}
	if len(parts) == 3 && parts[0] == "tenants" && parts[2] == "token" && request.Method == http.MethodPost {
		var input struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.UserID == "" {
			h.writeError(response, "user_id is required", http.StatusBadRequest)
			return
		}
		data, err := h.service.Token(request.Context(), parts[1], input.UserID)
		h.write(response, json.RawMessage(data), err, http.StatusOK)
		return
	}
	if len(parts) == 3 && parts[0] == "admin" && request.Method == http.MethodPost {
		var state AccountState
		switch parts[2] {
		case "suspend":
			state = AccountSuspended
		case "activate":
			state = AccountActive
		default:
			h.writeError(response, "unknown admin operation", http.StatusNotFound)
			return
		}
		account, err := h.service.SetState(request.Context(), parts[1], state)
		h.write(response, account, err, http.StatusOK)
		return
	}
	h.writeError(response, "route not found", http.StatusNotFound)
}

func (h *AdminHandler) write(response http.ResponseWriter, value any, err error, successStatus int) {
	if err == nil {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(successStatus)
		_ = json.NewEncoder(response).Encode(value)
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, ErrAccountSuspended) {
		status = http.StatusForbidden
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	} else if errors.As(err, &apiErr) {
		status = http.StatusBadGateway
	}
	h.writeError(response, err.Error(), status)
}

func (h *AdminHandler) writeError(response http.ResponseWriter, message string, status int) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]string{"error": message})
}
