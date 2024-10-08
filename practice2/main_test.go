package practice2

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
)

func TestInsertHandler(t *testing.T) {
	mux := http.NewServeMux()
	s := NewStorage(mux, "storage", []string{})
	s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	r.Run()
	defer r.Stop()

	feature := geojson.NewFeature(orb.Point{rand.Float64(), rand.Float64()})
	feature.ID = uuid.New().String()
	body, err := json.Marshal(feature)
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
	s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	r.Run()
	defer r.Stop()

	// Вставляем объект для проверки
	feature := geojson.NewFeature(orb.Point{1.0, 1.0})
	feature.ID = uuid.New().String()
	body, err := json.Marshal(feature)
	if err != nil {
		t.Fatal(err)
	}
	reqInsert, err := http.NewRequest("POST", "/insert", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rrInsert := httptest.NewRecorder()
	mux.ServeHTTP(rrInsert, reqInsert)
	if rrInsert.Code == http.StatusTemporaryRedirect {
		reqInsert, err := http.NewRequest("POST", rrInsert.Header().Get("Location"), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		rrInsert := httptest.NewRecorder()
		mux.ServeHTTP(rrInsert, reqInsert)
		if rrInsert.Code != http.StatusOK {
			t.Errorf("Insert handler returned wrong status code: got %v want %v", rrInsert.Code, http.StatusOK)
		}
	}

	req, err := http.NewRequest("GET", "/select?minX=0&minY=0&maxX=2&maxY=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusTemporaryRedirect {
		req, err := http.NewRequest("GET", rr.Header().Get("Location")+"?minX=0&minY=0&maxX=2&maxY=2", nil)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()

		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Select handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
		var fc geojson.FeatureCollection
		if err := json.Unmarshal(rr.Body.Bytes(), &fc); err != nil {
			t.Fatal(err)
		}
		if len(fc.Features) == 0 {
			t.Errorf("Select handler returned no features, expected at least one")
		}
	} else if rr.Code != http.StatusOK {
		t.Errorf("Select handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestReplaceHandler(t *testing.T) {
	mux := http.NewServeMux()
	s := NewStorage(mux, "storage", []string{})
	s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	r.Run()
	defer r.Stop()

	feature := geojson.NewFeature(orb.Point{rand.Float64(), rand.Float64()})
	feature.ID = uuid.New().String()
	body, err := json.Marshal(feature)
	if err != nil {
		t.Fatal(err)
	}
	// Вставляем объект
	reqInsert, err := http.NewRequest("POST", "/insert", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rrInsert := httptest.NewRecorder()
	mux.ServeHTTP(rrInsert, reqInsert)
	if rrInsert.Code == http.StatusTemporaryRedirect {
		reqInsert, err := http.NewRequest("POST", rrInsert.Header().Get("Location"), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		rrInsert := httptest.NewRecorder()
		mux.ServeHTTP(rrInsert, reqInsert)
		if rrInsert.Code != http.StatusOK {
			t.Errorf("Insert handler returned wrong status code: got %v want %v", rrInsert.Code, http.StatusOK)
		}
	}

	// Меняем координаты и делаем replace
	feature.Geometry = orb.Point{rand.Float64(), rand.Float64()}
	body, err = json.Marshal(feature)
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
			t.Errorf("Replace handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	} else if rr.Code != http.StatusOK {
		t.Errorf("Replace handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}

func TestDeleteHandler(t *testing.T) {
	mux := http.NewServeMux()

	s := NewStorage(mux, "storage", []string{})
	s.Run()
	defer s.Stop()

	r := NewRouter(mux, [][]string{{"storage"}})
	r.Run()
	defer r.Stop()

	feature := geojson.NewFeature(orb.Point{rand.Float64(), rand.Float64()})
	feature.ID = uuid.New().String()
	body, err := json.Marshal(feature)
	if err != nil {
		t.Fatal(err)
	}
	// Вставляем объект
	reqInsert, err := http.NewRequest("POST", "/insert", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rrInsert := httptest.NewRecorder()
	mux.ServeHTTP(rrInsert, reqInsert)
	if rrInsert.Code == http.StatusTemporaryRedirect {
		reqInsert, err := http.NewRequest("POST", rrInsert.Header().Get("Location"), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		rrInsert := httptest.NewRecorder()
		mux.ServeHTTP(rrInsert, reqInsert)
		if rrInsert.Code != http.StatusOK {
			t.Errorf("Insert handler returned wrong status code: got %v want %v", rrInsert.Code, http.StatusOK)
		}
	}

	// Удаляем объект
	req, err := http.NewRequest("POST", "/delete", bytes.NewReader(body))
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
			t.Errorf("Delete handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	} else if rr.Code != http.StatusOK {
		t.Errorf("Delete handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}
}
