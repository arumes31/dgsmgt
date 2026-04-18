package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"foo": "bar"}
	errMsg := "some error"
	meta := map[string]int{"count": 1}
	
	JSON(w, http.StatusTeapot, data, errMsg, meta)
	
	if w.Code != http.StatusTeapot {
		t.Errorf("Expected status %d, got %d", http.StatusTeapot, w.Code)
	}
	
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}
	
	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if resp.Error != errMsg {
		t.Errorf("Expected error %s, got %s", errMsg, resp.Error)
	}
}

func TestHelpers(t *testing.T) {
	tests := []struct {
		name       string
		fn         func(http.ResponseWriter)
		expected   int
	}{
		{"Success", func(w http.ResponseWriter) { Success(w, "ok") }, http.StatusOK},
		{"Created", func(w http.ResponseWriter) { Created(w, "ok") }, http.StatusCreated},
		{"BadRequest", func(w http.ResponseWriter) { BadRequest(w, "err") }, http.StatusBadRequest},
		{"Unauthorized", func(w http.ResponseWriter) { Unauthorized(w, "err") }, http.StatusUnauthorized},
		{"Forbidden", func(w http.ResponseWriter) { Forbidden(w, "err") }, http.StatusForbidden},
		{"NotFound", func(w http.ResponseWriter) { NotFound(w, "err") }, http.StatusNotFound},
		{"InternalError", func(w http.ResponseWriter) { InternalError(w, "err") }, http.StatusInternalServerError},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.fn(w)
			if w.Code != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, w.Code)
			}
		})
	}
}
