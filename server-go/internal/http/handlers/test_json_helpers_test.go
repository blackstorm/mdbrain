package handlers

import (
	"encoding/json"
	"testing"
)

func readJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal json: %v body=%s", err, string(raw))
	}
	return out
}

