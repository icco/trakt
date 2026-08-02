package trakt

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a client pointed at srv.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient("cid", "secret")
	c.BaseURL = srv.URL
	return c
}

func TestSyncParsesMoviesWithIDs(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("trakt-api-key"); got != "cid" {
			t.Errorf("trakt-api-key = %q, want cid", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		if got := r.Header.Get("trakt-api-version"); got != "2" {
			t.Errorf("trakt-api-version = %q, want 2", got)
		}
		if got := r.URL.Path; got != "/sync/ratings/movies" {
			t.Errorf("path = %q, want /sync/ratings/movies", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"rating":9,"movie":{"title":"The Matrix","year":1999,"ids":{"trakt":1,"imdb":"tt0133093","tmdb":603,"tvdb":0}}}]`))
	}))
	defer srv.Close()

	rows, err := newTestClient(srv).Sync(t.Context(), "tok", "sync/ratings/movies")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.Rating != 9 {
		t.Errorf("Rating = %d, want 9", row.Rating)
	}
	if row.Show != nil {
		t.Errorf("Show = %+v, want nil on a movie row", row.Show)
	}
	if row.Movie == nil {
		t.Fatal("Movie = nil, want populated")
	}
	if row.Movie.Title != "The Matrix" || row.Movie.Year != 1999 {
		t.Errorf("Movie = %+v", row.Movie)
	}
	if row.Movie.IDs.TMDb != 603 || row.Movie.IDs.IMDb != "tt0133093" || row.Movie.IDs.Trakt != 1 {
		t.Errorf("IDs = %+v", row.Movie.IDs)
	}
}

// Show rows and movie rows come back from the same endpoint shape, so a caller
// switching on which pointer is set must be able to trust it.
func TestSyncParsesShows(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"show":{"title":"Severance","year":2022,"ids":{"trakt":7,"tvdb":371980}}}]`))
	}))
	defer srv.Close()

	rows, err := newTestClient(srv).Sync(t.Context(), "tok", "sync/watched/shows")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(rows) != 1 || rows[0].Show == nil {
		t.Fatalf("rows = %+v, want one show row", rows)
	}
	if rows[0].Movie != nil {
		t.Errorf("Movie = %+v, want nil on a show row", rows[0].Movie)
	}
	if rows[0].Show.IDs.TVDb != 371980 {
		t.Errorf("TVDb = %d, want 371980", rows[0].Show.IDs.TVDb)
	}
}

func TestSyncEmptyList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	rows, err := newTestClient(srv).Sync(t.Context(), "tok", "sync/watchlist/movies")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestSyncSurfacesHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).Sync(t.Context(), "stale", "sync/watched/movies")
	if err == nil {
		t.Fatal("Sync on a 401 = nil, want an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not mention the status code", err)
	}
}

func TestSyncRejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not":"an array"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).Sync(t.Context(), "tok", "sync/watched/movies"); err == nil {
		t.Fatal("Sync on a non-array body = nil, want a decode error")
	}
}

func TestRequestDeviceCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/device/code" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var sent map[string]string
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if sent["client_id"] != "cid" {
			t.Errorf("client_id = %q, want cid", sent["client_id"])
		}
		// The device-code call is unauthenticated; the secret must not ride along.
		if _, ok := sent["client_secret"]; ok {
			t.Error("client_secret sent on the device-code request")
		}
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"1234","verification_url":"https://trakt.tv/activate","expires_in":600,"interval":5}`))
	}))
	defer srv.Close()

	dc, err := newTestClient(srv).RequestDeviceCode(t.Context())
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	if dc.DeviceCode != "dc" || dc.UserCode != "1234" {
		t.Errorf("got %+v", dc)
	}
	if dc.Interval != 5 || dc.ExpiresIn != 600 {
		t.Errorf("Interval/ExpiresIn = %d/%d, want 5/600", dc.Interval, dc.ExpiresIn)
	}
}

// Trakt answers an unapproved device code with HTTP 400. That is the normal
// state for most of the flow, so it must come back as (nil, nil) and not as an
// error the caller would abort on.
func TestPollForTokenPendingIsNotAnError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()

	tok, err := newTestClient(srv).PollForToken(t.Context(), "dc")
	if err != nil {
		t.Fatalf("PollForToken while pending = %v, want nil error", err)
	}
	if tok != nil {
		t.Errorf("token = %+v, want nil while pending", tok)
	}
}

func TestPollForTokenSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/device/token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var sent map[string]string
		_ = json.Unmarshal(body, &sent)
		if sent["code"] != "dc" || sent["client_secret"] != "secret" {
			t.Errorf("body = %v", sent)
		}
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":7776000,"created_at":1700000000}`))
	}))
	defer srv.Close()

	tok, err := newTestClient(srv).PollForToken(t.Context(), "dc")
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok == nil {
		t.Fatal("token = nil, want populated")
	}
	if tok.AccessToken != "at" || tok.RefreshToken != "rt" {
		t.Errorf("got %+v", tok)
	}
}

// A 4xx that is not 400 is a real failure (a revoked or mistyped device code),
// not the pending state, and must surface as an error.
func TestPollForTokenSurfacesNonPendingError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"invalid device code"}`))
	}))
	defer srv.Close()

	tok, err := newTestClient(srv).PollForToken(t.Context(), "bogus")
	if err == nil {
		t.Fatal("PollForToken on a 404 = nil, want an error")
	}
	if tok != nil {
		t.Errorf("token = %+v, want nil", tok)
	}
}

func TestRefreshToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var sent map[string]string
		_ = json.Unmarshal(body, &sent)
		if sent["grant_type"] != "refresh_token" || sent["refresh_token"] != "old" {
			t.Errorf("body = %v", sent)
		}
		_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","expires_in":7776000,"created_at":1700000000}`))
	}))
	defer srv.Close()

	tok, err := newTestClient(srv).RefreshToken(t.Context(), "old")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tok.AccessToken != "new-at" || tok.RefreshToken != "new-rt" {
		t.Errorf("got %+v", tok)
	}
}

func TestRefreshTokenSurfacesError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).RefreshToken(t.Context(), "revoked"); err == nil {
		t.Fatal("RefreshToken on a 401 = nil, want an error")
	}
}

func TestTokenExpiresAt(t *testing.T) {
	t.Parallel()
	tok := Token{CreatedAt: 1700000000, ExpiresIn: 7776000}
	want := time.Unix(1700000000, 0).Add(7776000 * time.Second)
	if got := tok.ExpiresAt(); !got.Equal(want) {
		t.Errorf("ExpiresAt() = %v, want %v", got, want)
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()
	c := NewClient("cid", "secret")
	if c.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, defaultBaseURL)
	}
	if c.httpClient == nil || c.httpClient.Timeout == 0 {
		t.Error("NewClient left the http client without a timeout")
	}
}
