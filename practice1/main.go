package practice1

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/paulmach/orb/geojson"
	"io/ioutil"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Router struct {
	mux   *http.ServeMux
	nodes [][]string
}

func NewRouter(mux *http.ServeMux, nodes [][]string) *Router {
	r := &Router{mux: mux, nodes: nodes}

	mux.Handle("/", http.FileServer(http.Dir("../front/dist")))
	mux.Handle("/select", http.RedirectHandler("/storage/select", http.StatusTemporaryRedirect))
	mux.Handle("/insert", http.RedirectHandler("/storage/insert", http.StatusTemporaryRedirect))
	mux.Handle("/replace", http.RedirectHandler("/storage/replace", http.StatusTemporaryRedirect))
	mux.Handle("/delete", http.RedirectHandler("/storage/delete", http.StatusTemporaryRedirect))
	return r
}

func (r *Router) Run() {

}

func (r *Router) Stop() {

}

type Storage struct {
	mux  *http.ServeMux // маршрутизатор для регистрации обработчиков запросов
	name string         // Имя данного узла хранения данных
}

func NewStorage(mux *http.ServeMux, name string, replicas []string) *Storage {
	s := &Storage{
		mux:  mux,
		name: name,
	}

	// Хэндлер для обработки GET-запроса на селект - возвращает данные из базы в формате JSON
	mux.HandleFunc("/"+name+"/select", func(w http.ResponseWriter, r *http.Request) {
		data, _ := ioutil.ReadFile("geo.db.json")
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Хэндлер для обработки POST-запроса на инсерт - принимает GeoJSON объект в теле запроса и добавляет его в базу данных
	mux.HandleFunc("/"+name+"/insert", func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		feature, _ := geojson.UnmarshalFeature(body)
		data, _ := ioutil.ReadFile("geo.db.json")
		collection, _ := geojson.UnmarshalFeatureCollection(data)
		collection.Append(feature)
		newData, _ := json.Marshal(collection)
		ioutil.WriteFile("geo.db.json", newData, 0644)
		w.WriteHeader(http.StatusOK)
	})

	// Хэндлер для обработки POST-запроса на замену объекта - принимает GeoJSON объект и заменяет существующий с таким же ID
	mux.HandleFunc("/"+name+"/replace", func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		feature, _ := geojson.UnmarshalFeature(body)
		data, _ := ioutil.ReadFile("geo.db.json")
		collection, _ := geojson.UnmarshalFeatureCollection(data)
		for i, f := range collection.Features {
			if f.ID == feature.ID {
				collection.Features[i] = feature
				break
			}
		}
		newData, _ := json.Marshal(collection)
		ioutil.WriteFile("geo.db.json", newData, 0644)
		w.WriteHeader(http.StatusOK)
	})

	// Хэндлер для обработки POST-запроса на удаление - принимает GeoJSON объект с ID и удаляет его из базы данных
	mux.HandleFunc("/"+name+"/delete", func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		feature, _ := geojson.UnmarshalFeature(body)
		data, _ := ioutil.ReadFile("geo.db.json")
		collection, _ := geojson.UnmarshalFeatureCollection(data)
		for i, f := range collection.Features {
			if f.ID == feature.ID {
				collection.Features = append(collection.Features[:i], collection.Features[i+1:]...)
				break
			}
		}
		newData, _ := json.Marshal(collection)
		ioutil.WriteFile("geo.db.json", newData, 0644)
		w.WriteHeader(http.StatusOK)
	})

	return s
}

func (r *Storage) Run() {

}

func (r *Storage) Stop() {

}

func main() {
	r := http.ServeMux{}

	router := NewRouter(&r)
	router.Run()

	storage := NewStorage(&r, "storage")
	storage.Run()

	l := http.Server{}
	l.Addr = "127.0.0.1:8080"
	l.Handler = &r

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		for _ = range sigs {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			l.Shutdown(ctx)
		}
	}()

	defer slog.Info("we are going down")
	slog.Info("listen http://" + l.Addr)
	err := l.ListenAndServe() // http event loop
	if !errors.Is(err, http.ErrServerClosed) {
		slog.Info("err", "err", err)
	}
}
