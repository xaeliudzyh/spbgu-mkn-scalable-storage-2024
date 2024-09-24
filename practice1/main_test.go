package practice1

import (
	"bytes"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimple(t *testing.T) {
	mux := http.NewServeMux()

	s := NewStorage(mux, "test", []string{}, true)
	go func() { s.Run() }()

	r := NewRouter(mux, [][]string{{"test"}})
	go func() { r.Run() }()
	t.Cleanup(r.Stop)

	t.Cleanup(s.Stop)

	feature := geojson.NewFeature(orb.Point{rand.Float64(), rand.Float64()})
	body, err := feature.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "/insert", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusTemporaryRedirect {
		req, err := http.NewRequest("POST", rr.Header().Get("location"), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	} else if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}
