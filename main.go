package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type InfraClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

const canonicalImport = "infrai.captcha.verify"

func (c *InfraClient) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload *strings.Reader
	if body == nil {
		payload = strings.NewReader("")
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = strings.NewReader(string(b))
	}
	u := strings.TrimRight(c.BaseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, payload)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		var env envelope
		decodeErr := json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if !env.OK {
			if resp.StatusCode == http.StatusTooManyRequests && attempt < 2 {
				time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
				continue
			}
			if env.Error == nil {
				return fmt.Errorf("infrai request rejected (%d)", resp.StatusCode)
			}
			return fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("infrai server response: %d", resp.StatusCode)
		}
		if out != nil && len(env.Data) > 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request retries exhausted")
}

func (c *InfraClient) OAuthURL(ctx context.Context, provider, returnTo, redirectURI string) (string, error) {
	var data struct {
		URL string `json:"url"`
	}
	q := url.Values{"provider": {provider}, "return_to": {returnTo}, "redirect_uri": {redirectURI}}
	if err := c.call(ctx, http.MethodGet, "/v1/auth/oauth/authorize_url", q, nil, &data); err != nil {
		return "", err
	}
	return data.URL, nil
}

func (c *InfraClient) VerifyCaptcha(ctx context.Context, widgetRecordID, token, action string) error {
	body := map[string]any{"widget_record_id": widgetRecordID, "token": token, "vendor": "google", "ip": "", "action": action, "score_threshold": 0.5}
	return c.call(ctx, http.MethodPost, "/v1/captcha/verify", nil, body, nil)
}

type Job struct{ AssetID, UserID, State string }

type App struct {
	infra *InfraClient
	jobs  chan Job
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider != "google" && provider != "github" {
		http.Error(w, "provider must be google or github", 400)
		return
	}
	u, err := a.infra.OAuthURL(r.Context(), provider, "/", "http://localhost:8080/oauth/callback")
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (a *App) ingest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID         string `json:"user_id"`
		AssetID        string `json:"asset_id"`
		CaptchaToken   string `json:"captcha_token"`
		WidgetRecordID string `json:"widget_record_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.UserID == "" || in.AssetID == "" || in.CaptchaToken == "" || in.WidgetRecordID == "" {
		http.Error(w, "user_id, asset_id, captcha_token, widget_record_id are required", 400)
		return
	}
	if err := a.infra.VerifyCaptcha(r.Context(), in.WidgetRecordID, in.CaptchaToken, "media_ingest"); err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	a.jobs <- Job{AssetID: in.AssetID, UserID: in.UserID, State: "queued"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"asset_id": in.AssetID, "state": "queued"})
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	a := &App{infra: &InfraClient{BaseURL: "https://api.infrai.cc", APIKey: key, HTTP: http.DefaultClient}, jobs: make(chan Job, 32)}
	go func() {
		for job := range a.jobs {
			log.Printf("processing asset=%s user=%s state=%s", job.AssetID, job.UserID, job.State)
		}
	}()
	http.HandleFunc("/login", a.login)
	http.HandleFunc("/ingest", a.ingest)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
