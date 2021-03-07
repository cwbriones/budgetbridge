package splitwise

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalSlice(t *testing.T) {
	payload := []byte(`
	{"errors" : ["invalid request"]}
	`)
	var response struct {
		Errors APIError `json:"errors"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Errorf("json: %s", err)
	}
	t.Logf("%s", response.Errors.Error())
	if l := response.Errors.Len(); l != 1 {
		t.Errorf("expected %d errs, got %d", 1, l)
	}
}

func TestUnmarshalMap(t *testing.T) {
	payload := []byte(`
	{"errors":{"base":["Invalid API Request: you do not have permission to perform that action"]}}
	`)
	var response struct {
		Errors APIError `json:"errors"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Errorf("json: %s", err)
	}
	t.Logf("%s", response.Errors.Error())
	if l := response.Errors.Len(); l != 1 {
		t.Errorf("expected %d errs, got %d", 1, l)
	}
}
