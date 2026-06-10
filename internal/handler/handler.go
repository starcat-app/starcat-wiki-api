package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dong4j/starcat-wiki-api/internal/model"
)

func writeJSON[T any](w http.ResponseWriter, data T) {
	writeJSONWithMeta(w, data, nil)
}

func writeJSONWithMeta[T any](w http.ResponseWriter, data T, meta *model.Meta) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	env := model.Envelope[T]{
		SchemaVersion: 1,
		Data:          data,
		Meta:          meta,
	}
	if err := json.NewEncoder(w).Encode(env); err != nil {
		log.Printf("[handler] failed to encode envelope: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string, details interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	env := model.ErrorEnvelope{
		SchemaVersion: 1,
		Error: model.ErrorResponse{
			Code:    code,
			Message: msg,
			Details: details,
		},
	}
	if err := json.NewEncoder(w).Encode(env); err != nil {
		log.Printf("[handler] failed to encode error envelope: %v", err)
	}
}
