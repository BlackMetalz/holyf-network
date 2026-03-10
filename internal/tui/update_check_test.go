package tui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCheckForUpdateWithURLAvailable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.33"}`))
	}))
	defer srv.Close()

	latest, ok := checkForUpdateWithURL(context.Background(), srv.Client(), srv.URL, "v0.3.32")
	if !ok {
		t.Fatalf("expected update available")
	}
	if latest != "v0.3.33" {
		t.Fatalf("latest mismatch: got=%q want=%q", latest, "v0.3.33")
	}
}

func TestCheckForUpdateWithURLUpToDate(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.32"}`))
	}))
	defer srv.Close()

	latest, ok := checkForUpdateWithURL(context.Background(), srv.Client(), srv.URL, "v0.3.32")
	if ok {
		t.Fatalf("expected no update, got latest=%q", latest)
	}
	if latest != "" {
		t.Fatalf("latest should be empty when no update, got=%q", latest)
	}
}

func TestCheckForUpdateWithURLFallbackOnErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		url    string
		client *http.Client
		server *httptest.Server
	}{
		{
			name: "non-200 status",
			server: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})),
		},
		{
			name: "invalid json",
			server: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"tag_name":`))
			})),
		},
		{
			name: "network error",
			url:  "http://example.invalid",
			client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			})},
		},
		{
			name: "timeout",
			server: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(200 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"tag_name":"v0.3.33"}`))
			})),
			client: &http.Client{Timeout: 50 * time.Millisecond},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			url := tc.url
			if tc.server != nil {
				defer tc.server.Close()
				url = tc.server.URL
			}
			client := tc.client
			if client == nil {
				if tc.server != nil {
					client = tc.server.Client()
				} else {
					client = &http.Client{}
				}
			}

			latest, ok := checkForUpdateWithURL(context.Background(), client, url, "v0.3.32")
			if ok {
				t.Fatalf("expected fallback no-update, got latest=%q", latest)
			}
			if latest != "" {
				t.Fatalf("latest should be empty on fallback, got=%q", latest)
			}
		})
	}
}
