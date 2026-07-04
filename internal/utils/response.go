package utils

import (
	"encoding/json"
	"net/http"
)

// Response is a standardized JSON response wrapper
type Response struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
	Meta  interface{} `json:"meta,omitempty"`
}

// JSON sends a standardized JSON response
func JSON(w http.ResponseWriter, statusCode int, data interface{}, err string, meta interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := Response{
		Data:  data,
		Error: err,
		Meta:  meta,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// Success sends a successful JSON response
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data, "", nil)
}

// Created sends a 201 Created JSON response
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, data, "", nil)
}

// BadRequest sends a 400 Bad Request JSON response
func BadRequest(w http.ResponseWriter, message string) {
	JSON(w, http.StatusBadRequest, nil, message, nil)
}

// Unauthorized sends a 401 Unauthorized JSON response
func Unauthorized(w http.ResponseWriter, message string) {
	JSON(w, http.StatusUnauthorized, nil, message, nil)
}

// Forbidden sends a 403 Forbidden JSON response
func Forbidden(w http.ResponseWriter, message string) {
	JSON(w, http.StatusForbidden, nil, message, nil)
}

// NotFound sends a 404 Not Found JSON response
func NotFound(w http.ResponseWriter, message string) {
	JSON(w, http.StatusNotFound, nil, message, nil)
}

// InternalError sends a 500 Internal Server Error JSON response
func InternalError(w http.ResponseWriter, message string) {
	JSON(w, http.StatusInternalServerError, nil, message, nil)
}
