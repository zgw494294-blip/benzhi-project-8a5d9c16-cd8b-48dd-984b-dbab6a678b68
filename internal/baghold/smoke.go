package baghold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

func RunSmoke(out io.Writer) error {
	store := NewStore()
	handler := NewHandler(store)

	created, err := smokeRequest(handler, http.MethodPost, "/tests", `{"bag_id":"SMOKE-001","min_hold_seconds":60,"max_vacuum_loss_kpa":2.5,"operator_note":"bench check"}`)
	if err != nil {
		return err
	}
	var record HoldTest
	if err := json.Unmarshal(created, &record); err != nil {
		return fmt.Errorf("decode created test: %w", err)
	}
	for _, body := range []string{
		`{"elapsed_seconds":0,"vacuum_kpa":30}`,
		`{"elapsed_seconds":60,"vacuum_kpa":28.5}`,
	} {
		if _, err := smokeRequest(handler, http.MethodPost, "/tests/"+record.ID+"/samples", body); err != nil {
			return err
		}
	}
	assessed, err := smokeRequest(handler, http.MethodPost, "/tests/"+record.ID+"/assess", "{}")
	if err != nil {
		return err
	}
	var completed HoldTest
	if err := json.Unmarshal(assessed, &completed); err != nil {
		return fmt.Errorf("decode assessment: %w", err)
	}
	retrieved, err := smokeRequest(handler, http.MethodGet, "/tests/"+record.ID, "")
	if err != nil {
		return err
	}
	var fetched HoldTest
	if err := json.Unmarshal(retrieved, &fetched); err != nil {
		return fmt.Errorf("decode retrieved test: %w", err)
	}
	if fetched.Status != StatusPassed || completed.Status != StatusPassed || fetched.Assessment == nil {
		return fmt.Errorf("smoke assessment did not pass")
	}
	_, err = fmt.Fprintf(out, "smoke: %s (%s, loss %.1f kPa)\n", fetched.Status, fetched.ID, fetched.Assessment.VacuumLossKPa)
	return err
}

func smokeRequest(handler http.Handler, method, path, body string) ([]byte, error) {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s %s returned HTTP %d: %s", method, path, response.Code, response.Body.String())
	}
	return response.Body.Bytes(), nil
}
