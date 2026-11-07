package nango

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

type Adapter struct {
	host   string
	key    string
	client Client
}

func New(host, key string) *Adapter {
	host = strings.TrimRight(host, "/")
	if host == "" {
		host = "https://api.nango.dev"
	}
	return &Adapter{host: host, key: key, client: &http.Client{Timeout: 12 * time.Second}}
}

type Connection struct {
	ConnectionID      string            `json:"connection_id"`
	ProviderConfigKey string            `json:"provider_config_key"`
	Provider          string            `json:"provider"`
	Tags              map[string]string `json:"tags"`
	Created           string            `json:"created"`
}

type Grant struct {
	ConnectLink string `json:"connect_link"`
	ExpiresAt   string `json:"expires_at"`
}

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("nango: HTTP %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("nango: HTTP %d", e.Status)
}

func (a *Adapter) request(method, path string, query url.Values, body any) (out []byte, err error) {
	u, err := url.Parse(a.host + path)
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	payload := strings.NewReader("")
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, u.String(), payload)
	if err != nil {
		return nil, err
	}
	if a.key != "" {
		req.Header.Set("Authorization", "Bearer "+a.key)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	out, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(out, &e); err != nil {
			return nil, err
		}
		return nil, &Error{Status: resp.StatusCode, Code: e.Error.Code}
	}
	return out, nil
}

func (a *Adapter) CreateGrant(tenant, provider string) (Grant, error) {
	out, err := a.request(http.MethodPost, "/connect/sessions", nil, map[string]any{
		"tags":                 map[string]string{"turvez_client": tenant},
		"allowed_integrations": []string{provider},
	})
	if err != nil {
		return Grant{}, err
	}
	var doc struct {
		Data struct {
			ConnectLink string `json:"connect_link"`
			ExpiresAt   string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return Grant{}, err
	}
	if doc.Data.ConnectLink == "" {
		return Grant{}, fmt.Errorf("nango: connect session returned no link")
	}
	return Grant{ConnectLink: doc.Data.ConnectLink, ExpiresAt: doc.Data.ExpiresAt}, nil
}

func (a *Adapter) ListForTenant(tenant string) ([]Connection, error) {
	out, err := a.request(http.MethodGet, "/connections", nil, nil)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Connections []Connection `json:"connections"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var out2 []Connection
	for _, c := range doc.Connections {
		if c.Tags != nil && c.Tags["turvez_client"] == tenant {
			out2 = append(out2, c)
		}
	}
	return out2, nil
}

func (a *Adapter) Revoke(connectionID, providerConfigKey string) error {
	q := url.Values{"provider_config_key": []string{providerConfigKey}}
	_, err := a.request(http.MethodDelete, "/connections/"+url.PathEscape(connectionID), q, nil)
	return err
}
