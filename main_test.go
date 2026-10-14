package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIngestQueuesVerifiedAsset(t *testing.T) {
	tests := []struct{ name, token, wantStatus, wantState string }{
		{"verified media", "ok-token", "200", "queued"},
		{"rejected media", "bad-token", "422", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["widget_record_id"] != "widget-1" {
					t.Errorf("widget_record_id=%v", body["widget_record_id"])
				}
				ok := body["token"] == "ok-token"
				w.Header().Set("Content-Type", "application/json")
				if ok {
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{}})
				} else {
					w.WriteHeader(422)
					json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]string{"code": "captcha_rejected", "message": "captcha rejected"}})
				}
			}))
			defer upstream.Close()
			a := &App{infra: &InfraClient{BaseURL: upstream.URL, APIKey: "test", HTTP: upstream.Client()}, jobs: make(chan Job, 1)}
			req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(`{"user_id":"u1","asset_id":"a1","widget_record_id":"widget-1","captcha_token":"`+tc.token+`"}`))
			rr := httptest.NewRecorder()
			a.ingest(rr, req)
			if rr.Code != statusCode(tc.wantStatus) {
				t.Fatalf("status=%d", rr.Code)
			}
			if tc.wantState != "" {
				var got map[string]string
				json.NewDecoder(rr.Body).Decode(&got)
				if got["state"] != tc.wantState {
					t.Fatalf("state=%q", got["state"])
				}
			}
		})
	}
	_ = context.Background()
}
func statusCode(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
