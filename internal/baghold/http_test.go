package baghold

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPWorkflowAndStrictErrors(t *testing.T) {
	handler := NewHandler(NewStore())

	created := requestJSON(t, handler, http.MethodPost, "/tests", `{"bag_id":"HTTP-1","min_hold_seconds":60,"max_vacuum_loss_kpa":2}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", created.Code, http.StatusCreated)
	}
	var record HoldTest
	decodeResponse(t, created, &record)
	if record.Status != StatusActive || len(record.Samples) != 0 {
		t.Fatalf("unexpected active record: %#v", record)
	}

	badJSON := requestJSON(t, handler, http.MethodPost, "/tests", `{"bag_id":"HTTP-2","min_hold_seconds":60,"max_vacuum_loss_kpa":2,"extra":true}`)
	assertError(t, badJSON, http.StatusBadRequest, string(ErrorInvalidInput))
	missingLimit := requestJSON(t, handler, http.MethodPost, "/tests", `{"bag_id":"HTTP-3","min_hold_seconds":60}`)
	assertError(t, missingLimit, http.StatusBadRequest, string(ErrorInvalidInput))

	first := requestJSON(t, handler, http.MethodPost, "/tests/"+record.ID+"/samples", `{"elapsed_seconds":0,"vacuum_kpa":30}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first sample status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := requestJSON(t, handler, http.MethodPost, "/tests/"+record.ID+"/samples", `{"elapsed_seconds":60,"vacuum_kpa":28.5}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("second sample status = %d, want %d", second.Code, http.StatusCreated)
	}
	assessment := requestJSON(t, handler, http.MethodPost, "/tests/"+record.ID+"/assess", `{}`)
	if assessment.Code != http.StatusOK {
		t.Fatalf("assessment status = %d, want %d", assessment.Code, http.StatusOK)
	}
	var assessed HoldTest
	decodeResponse(t, assessment, &assessed)
	if assessed.Status != StatusPassed || assessed.Assessment == nil || assessed.Assessment.VacuumLossKPa != 1.5 {
		t.Fatalf("unexpected assessment: %#v", assessed)
	}
	malformedAssessment := requestJSON(t, handler, http.MethodPost, "/tests/"+record.ID+"/assess", `{"extra":true}`)
	assertError(t, malformedAssessment, http.StatusBadRequest, string(ErrorInvalidInput))

	conflict := requestJSON(t, handler, http.MethodPost, "/tests/"+record.ID+"/samples", `{"elapsed_seconds":61,"vacuum_kpa":28}`)
	assertError(t, conflict, http.StatusConflict, string(ErrorConflict))
	missing := requestJSON(t, handler, http.MethodGet, "/tests/missing", "")
	assertError(t, missing, http.StatusNotFound, string(ErrorNotFound))

	got := requestJSON(t, handler, http.MethodGet, "/tests/"+record.ID, "")
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", got.Code, http.StatusOK)
	}
	var fetched HoldTest
	decodeResponse(t, got, &fetched)
	if fetched.Status != StatusPassed || len(fetched.Samples) != 2 {
		t.Fatalf("unexpected fetched record: %#v", fetched)
	}
}

func TestHTTPPreservesOmittedAndEmptyNotes(t *testing.T) {
	handler := NewHandler(NewStore())
	omitted := requestJSON(t, handler, http.MethodPost, "/tests", `{"bag_id":"OMITTED","min_hold_seconds":1,"max_vacuum_loss_kpa":1}`)
	empty := requestJSON(t, handler, http.MethodPost, "/tests", `{"bag_id":"EMPTY","min_hold_seconds":1,"max_vacuum_loss_kpa":1,"operator_note":""}`)
	for name, response := range map[string]*httptest.ResponseRecorder{"omitted": omitted, "empty": empty} {
		if response.Code != http.StatusCreated {
			t.Fatalf("%s create status = %d", name, response.Code)
		}
	}
	var omittedFields map[string]json.RawMessage
	var emptyFields map[string]json.RawMessage
	decodeResponse(t, omitted, &omittedFields)
	decodeResponse(t, empty, &emptyFields)
	if _, ok := omittedFields["operator_note"]; ok {
		t.Fatalf("omitted note appeared in response: %s", omitted.Body.String())
	}
	value, ok := emptyFields["operator_note"]
	if !ok || string(value) != `""` {
		t.Fatalf("empty note was not preserved in response: %s", empty.Body.String())
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("error status = %d, want %d: %s", response.Code, status, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, response, &payload)
	if payload.Error.Code != code {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, code)
	}
}
