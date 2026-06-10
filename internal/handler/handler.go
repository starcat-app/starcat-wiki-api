package handler

import (
	"encoding/json"
	"net/http"

	"github.com/dong4j/starcat-wiki-api/internal/model"
)

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(model.Envelope[interface{}]{
		SchemaVersion: 1,
		Data:          data,
	})
}

func writeJSONWithMeta(w http.ResponseWriter, data interface{}, meta *model.Meta) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(model.Envelope[interface{}]{
		SchemaVersion: 1,
		Data:          data,
		Meta:          meta,
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string, details interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.ErrorEnvelope{
		SchemaVersion: 1,
		Error: model.ErrorResponse{
			Code:    code,
			Message: msg,
			Details: details,
		},
	})
}
