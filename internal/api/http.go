package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func (api handler) writeError(response http.ResponseWriter, status int, code, message string) {
	api.writeJSON(response, status, errorResponse{Code: code, Message: message})
}

func (api handler) methodNotAllowed(allow string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		api.logger.Warn("method not allowed", "method", request.Method, "path", request.URL.Path)
		response.Header().Set("Allow", allow)
		api.writeError(response, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (api handler) notFound(response http.ResponseWriter, request *http.Request) {
	api.logger.Warn("route not found", "method", request.Method, "path", request.URL.Path)
	api.writeError(response, http.StatusNotFound, "not_found", "route not found")
}

func (api handler) writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(body); err != nil {
		api.logger.Error("encode response", "error", err)
	}
}
