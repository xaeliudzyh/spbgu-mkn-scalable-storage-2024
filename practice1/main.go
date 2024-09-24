package practice1

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Router struct {
	mux   *http.ServeMux
	nodes [][]string
	stop  chan struct{} // канал для остановки роутера
}

func NewRouter(mux *http.ServeMux, nodes [][]string) *Router {
	r := &Router{mux: mux, nodes: nodes, stop: make(chan struct{})}

	mux.Handle("/", http.FileServer(http.Dir("../front/dist")))
	mux.Handle("/select", http.RedirectHandler("/storage/select", http.StatusTemporaryRedirect))
	mux.Handle("/insert", http.RedirectHandler("/storage/insert", http.StatusTemporaryRedirect))
	mux.Handle("/replace", http.RedirectHandler("/storage/replace", http.StatusTemporaryRedirect))
	mux.Handle("/delete", http.RedirectHandler("/storage/delete", http.StatusTemporaryRedirect))
	return r
}

// Run запускает сервис Router
func (r *Router) Run() {
	r.stop = make(chan struct{}) // создаём канал для остановки
	go func() {
		<-r.stop
	}()

	log.Println("Router запущен") // логируем запуск сервиса
}

// Stop останавливает сервис Router
func (r *Router) Stop() {
	// Отправляем сигнал для завершения работы через канал
	if r.stop != nil {
		close(r.stop)                    // Закрываем только если канал был инициализирован
		log.Println("Router остановлен") // логируем остановку сервиса
	}
}

type Storage struct {
	mux  *http.ServeMux
	name string
	stop chan struct{} // канал для остановки сервиса
}

func NewStorage(mux *http.ServeMux, name string, replicas []string) *Storage {
	s := &Storage{
		mux:  mux,
		name: name,
	}

	// Хэндлер для обработки запроса select, возвращает пустой geojson объект FeatureCollection
	mux.HandleFunc("/"+name+"/select", func(w http.ResponseWriter, r *http.Request) {
		data := []byte(`{"type": "FeatureCollection", "features": []}`)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Хэндлер для обработки запроса insert, возвращает OK без body
	mux.HandleFunc("/"+name+"/insert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Хэндлер для обработки запроса replace, возвращает OK без body
	mux.HandleFunc("/"+name+"/replace", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Хэндлер для обработки запроса delete, возвращает OK без body
	mux.HandleFunc("/"+name+"/delete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return s
}

// Run запускает Storage сервис
func (s *Storage) Run() {
	s.stop = make(chan struct{}) // создаём канал для остановки

	// Запускаем сервис в горутине
	go func() {
		<-s.stop
	}()

	log.Printf("Storage '%s' запущен", s.name) // логируем запуск сервиса
}

// Stop останавливает Storage сервис
func (s *Storage) Stop() {
	// Отправляем сигнал в канал для остановки работы
	if s.stop != nil {
		close(s.stop)
		log.Printf("Storage '%s' остановлен", s.name) // логируем остановку сервиса
	}
}

func main() {
	r := http.ServeMux{}
	nodes := [][]string{}
	router := NewRouter(&r, nodes)
	router.Run()

	storage := NewStorage(&r, "storage", nil)
	storage.Run()

	// Настраиваем HTTP сервер
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: &r,
	}

	// Канал для получения системных сигналов
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Запуск сервера в отдельной горутине
	go func() {
		log.Printf("Server is listening on %s", server.Addr)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	sig := <-sigs
	log.Printf("Received signal: %v, shutting down server...", sig)

	// Останавливаем сервер с таймаутом 5 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	router.Stop()
	storage.Stop()

	log.Println("Server stopped gracefully")
}
