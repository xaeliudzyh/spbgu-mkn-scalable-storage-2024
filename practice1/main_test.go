package practice1

import (
	"bytes"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInsertHandler(t *testing.T) {
	mux := http.NewServeMux()
	s := NewStorage(mux, "storage", []string{})
	go s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	go r.Run()
	defer r.Stop()
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
		req, err := http.NewRequest("POST", rr.Header().Get("Location"), bytes.NewReader(body))
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

func TestSelectHandler(t *testing.T) {
	mux := http.NewServeMux()
	s := NewStorage(mux, "storage", []string{})
	go s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	go r.Run()
	defer r.Stop()
	req, err := http.NewRequest("GET", "/select", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusTemporaryRedirect {
		req, err := http.NewRequest("GET", rr.Header().Get("Location"), nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
		expected := `{"type": "FeatureCollection", "features": []}`
		if rr.Body.String() != expected {
			t.Errorf("handler returned wrong body: got %v want %v", rr.Body.String(), expected)
		}
	} else if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestReplaceHandler(t *testing.T) {
	mux := http.NewServeMux()
	s := NewStorage(mux, "storage", []string{})
	go s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	go r.Run()
	defer r.Stop()
	feature := geojson.NewFeature(orb.Point{rand.Float64(), rand.Float64()})
	body, err := feature.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "/replace", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusTemporaryRedirect {
		req, err := http.NewRequest("POST", rr.Header().Get("Location"), bytes.NewReader(body))
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

func TestDeleteHandler(t *testing.T) {
	mux := http.NewServeMux()

	s := NewStorage(mux, "storage", []string{})
	go s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	go r.Run()
	defer r.Stop()

	req, err := http.NewRequest("DELETE", "/delete", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusTemporaryRedirect {
		req, err := http.NewRequest("DELETE", rr.Header().Get("Location"), nil)
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
