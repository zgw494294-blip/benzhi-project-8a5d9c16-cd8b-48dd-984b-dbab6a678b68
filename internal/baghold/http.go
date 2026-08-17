package baghold

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type createRequest struct {
	BagID                *string         `json:"bag_id"`
	MinimumHoldSeconds   *float64        `json:"min_hold_seconds"`
	MaximumVacuumLossKPa *float64        `json:"max_vacuum_loss_kpa"`
	OperatorNote         json.RawMessage `json:"operator_note"`
}

type sampleRequest struct {
	ElapsedSeconds *float64 `json:"elapsed_seconds"`
	VacuumKPa      *float64 `json:"vacuum_kpa"`
}

type Handler struct {
	store *Store
}

func NewHandler(store *Store) http.Handler {
	if store == nil {
		store = NewStore()
	}
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 1 && parts[0] == "tests" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		h.createTest(w, r)
		return
	}
	if len(parts) != 2 && len(parts) != 3 || parts[0] != "tests" || parts[1] == "" {
		writeError(w, http.StatusNotFound, string(ErrorNotFound), "route not found")
		return
	}

	id := parts[1]
	switch {
	case len(parts) == 2:
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		h.getTest(w, id)
	case parts[2] == "samples":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		h.addSample(w, r, id)
	case parts[2] == "assess":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		h.assess(w, r, id)
	default:
		writeError(w, http.StatusNotFound, string(ErrorNotFound), "route not found")
	}
}

func (h *Handler) createTest(w http.ResponseWriter, r *http.Request) {
	var request createRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	input, err := request.input()
	if err != nil {
		writeDomainError(w, err)
		return
	}
	record, err := h.store.Create(r.Context(), input)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) addSample(w http.ResponseWriter, r *http.Request, id string) {
	var request sampleRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	input, err := request.input()
	if err != nil {
		writeDomainError(w, err)
		return
	}
	record, err := h.store.AddSample(r.Context(), id, input)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) assess(w http.ResponseWriter, r *http.Request, id string) {
	if err := decodeOptionalObject(r); err != nil {
		writeDecodeError(w, err)
		return
	}
	record, err := h.store.Assess(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) getTest(w http.ResponseWriter, id string) {
	record, err := h.store.Get(id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func (r createRequest) input() (CreateInput, error) {
	if r.BagID == nil {
		return CreateInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "bag_id", "is required")
	}
	if r.MinimumHoldSeconds == nil {
		return CreateInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "min_hold_seconds", "is required")
	}
	if r.MaximumVacuumLossKPa == nil {
		return CreateInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "max_vacuum_loss_kpa", "is required")
	}
	var note *string
	if r.OperatorNote != nil {
		if string(r.OperatorNote) == "null" {
			return CreateInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "operator_note", "must be a string")
		}
		var value string
		if err := json.Unmarshal(r.OperatorNote, &value); err != nil {
			return CreateInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "operator_note", "must be a string")
		}
		note = &value
	}
	return CreateInput{
		BagID:                *r.BagID,
		MinimumHoldSeconds:   *r.MinimumHoldSeconds,
		MaximumVacuumLossKPa: *r.MaximumVacuumLossKPa,
		OperatorNote:         note,
	}, nil
}

func (r sampleRequest) input() (SampleInput, error) {
	if r.ElapsedSeconds == nil {
		return SampleInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "elapsed_seconds", "is required")
	}
	if r.VacuumKPa == nil {
		return SampleInput{}, domainError(ErrorInvalidInput, ErrInvalidInput, "vacuum_kpa", "is required")
	}
	return SampleInput{ElapsedSeconds: *r.ElapsedSeconds, VacuumKPa: *r.VacuumKPa}, nil
}

func decodeOptionalObject(r *http.Request) error {
	decoder := json.NewDecoder(r.Body)
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	if object == nil {
		return errors.New("request body must contain a JSON object")
	}
	if len(object) != 0 {
		return errors.New("request body contains an unknown field")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	message := "request body must be valid JSON"
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		message = "request body contains an invalid value"
	}
	writeError(w, http.StatusBadRequest, string(ErrorInvalidInput), message)
}

func writeDomainError(w http.ResponseWriter, err error) {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Kind {
		case ErrorInvalidInput:
			writeError(w, http.StatusBadRequest, string(ErrorInvalidInput), domainErr.Error())
		case ErrorNotFound:
			writeError(w, http.StatusNotFound, string(ErrorNotFound), domainErr.Message)
		case ErrorConflict:
			writeError(w, http.StatusConflict, string(ErrorConflict), domainErr.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
