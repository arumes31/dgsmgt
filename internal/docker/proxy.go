package docker

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

func NewProxyHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status/{id}", func(w http.ResponseWriter, r *http.Request) {
		info, err := service.GetStatus(r.Context(), r.PathValue("id"))
		writeProxyJSON(w, info, err)
	})
	mux.HandleFunc("POST /v1/start/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeProxyJSON(w, nil, service.Start(r.Context(), r.PathValue("id")))
	})
	mux.HandleFunc("POST /v1/stop/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeProxyJSON(w, nil, service.Stop(r.Context(), r.PathValue("id")))
	})
	mux.HandleFunc("POST /v1/restart/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeProxyJSON(w, nil, service.Restart(r.Context(), r.PathValue("id")))
	})
	mux.HandleFunc("GET /v1/logs/{id}", func(w http.ResponseWriter, r *http.Request) {
		stream, err := service.Logs(r.Context(), r.PathValue("id"), r.URL.Query().Get("tail"))
		streamProxyResponse(w, stream, err)
	})
	mux.HandleFunc("GET /v1/stats/{id}", func(w http.ResponseWriter, r *http.Request) {
		stream, err := service.Stats(r.Context(), r.PathValue("id"))
		streamProxyResponse(w, stream, err)
	})
	mux.HandleFunc("POST /v1/containers", func(w http.ResponseWriter, r *http.Request) {
		var input createRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		id, err := service.Create(
			r.Context(),
			input.Name,
			input.Image,
			input.Ports,
			input.Env,
			input.Volumes,
		)
		writeProxyJSON(w, createResponse{ID: id}, err)
	})
	mux.HandleFunc("DELETE /v1/containers/{id}", func(w http.ResponseWriter, r *http.Request) {
		force, err := strconv.ParseBool(r.URL.Query().Get("force"))
		if err != nil {
			http.Error(w, "invalid force value", http.StatusBadRequest)
			return
		}
		writeProxyJSON(w, nil, service.Delete(r.Context(), r.PathValue("id"), force))
	})
	mux.HandleFunc("GET /v1/containers", func(w http.ResponseWriter, r *http.Request) {
		containers, err := service.List(r.Context())
		writeProxyJSON(w, containers, err)
	})
	return mux
}

func writeProxyJSON(w http.ResponseWriter, value any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrCreationDenied):
			status = http.StatusBadRequest
		case errors.Is(err, ErrUnmanagedContainer):
			status = http.StatusForbidden
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	if value == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "encoding response", http.StatusInternalServerError)
	}
}

func streamProxyResponse(w http.ResponseWriter, stream io.ReadCloser, err error) {
	if err != nil {
		writeProxyJSON(w, nil, err)
		return
	}
	defer func() { _ = stream.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, stream)
}
