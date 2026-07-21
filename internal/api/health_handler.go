package api

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

func (api handler) health(response http.ResponseWriter, _ *http.Request) {
	api.writeJSON(response, http.StatusOK, healthResponse{Status: "ok"})
}
