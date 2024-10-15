package practice2

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/tidwall/rtree"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

type Checkpoint struct {
	Data   map[string]*geojson.Feature `json:"data"`
	VClock map[string]uint64           `json:"vclock"`
}

type Transaction struct {
	Action  string           `json:"action"`
	Name    string           `json:"name"`
	LSN     uint64           `json:"lsn"`
	Feature *geojson.Feature `json:"feature"`
}

type Command struct {
	action       string
	feature      *geojson.Feature
	min          [2]float64
	max          [2]float64
	result       chan error
	searchResult chan SearchResult
}

type SearchResult struct {
	Features []*geojson.Feature
	Error    error
}

type Engine struct {
	data       map[string]*geojson.Feature
	spatialIdx *rtree.RTree
	lsn        uint64
	name       string
	ctx        context.Context
	cancel     context.CancelFunc
	commands   chan Command

	vclock   map[string]uint64
	replicas []string
	leader   bool
	// Реестр подключенных реплик
	replicaConns map[string]*websocket.Conn
}

func (e *Engine) applyTransaction(txn *Transaction) {
	// Проверяем, применяли ли мы уже эту транзакцию
	lastLSN, exists := e.vclock[txn.Name]
	if exists && txn.LSN <= lastLSN {
		return // Уже применили эту или более новую транзакцию от этого узла
	}
	// Обновляем vclock
	e.vclock[txn.Name] = txn.LSN
	// Применяем транзакцию
	switch txn.Action {
	case "insert", "replace":
		idStr, ok := txn.Feature.ID.(string)
		if !ok {
			return
		}
		e.data[idStr] = txn.Feature
		minX, minY, maxX, maxY := getBoundingBox(txn.Feature.Geometry)
		e.spatialIdx.Insert([2]float64{minX, minY}, [2]float64{maxX, maxY}, txn.Feature)
	case "delete":
		idStr, ok := txn.Feature.ID.(string)
		if !ok {
			return
		}
		feature, exists := e.data[idStr]
		if !exists {
			return
		}
		minX, minY, maxX, maxY := getBoundingBox(feature.Geometry)
		e.spatialIdx.Delete([2]float64{minX, minY}, [2]float64{maxX, maxY}, feature)
		delete(e.data, idStr)
	}
}

func (e *Engine) connectToReplicas() {
	for _, addr := range e.replicas {
		go func(addr string) {
			// Установить соединение с репликой по WebSocket
			conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/replication", nil)
			if err != nil {
				log.Printf("Ошибка подключения к реплике %s: %v", addr, err)
				return
			}
			e.replicaConns[addr] = conn

			// Слушаем входящие сообщения от реплики
			for {
				var txn Transaction
				err := conn.ReadJSON(&txn)
				if err != nil {
					log.Printf("Ошибка чтения транзакции от реплики %s: %v", addr, err)
					conn.Close()
					delete(e.replicaConns, addr)
					return
				}
				e.applyTransaction(&txn)
			}
		}(addr)
	}
}

func (e *Engine) Run() {
	go func() {
		// Загрузка чекпоинта и воспроизведение транзакций
		if err := e.loadCheckpoint(); err != nil {
			log.Printf("Ошибка загрузки чекпоинта: %v", err)
		}
		if err := e.replayTransactions(); err != nil {
			log.Printf("Ошибка воспроизведения транзакций: %v", err)
		}
		e.connectToReplicas()
		for {
			select {
			case <-e.ctx.Done():
				// Перед завершением сохраняем чекпоинт
				if err := e.checkpoint(); err != nil {
					log.Printf("Ошибка при создании чекпоинта: %v", err)
				}
				return
			case cmd := <-e.commands:
				e.handleCommand(cmd)
			}
		}
	}()
}

func (e *Engine) broadcastTransaction(txn *Transaction) {
	for addr, conn := range e.replicaConns {
		err := conn.WriteJSON(txn)
		if err != nil {
			log.Printf("Ошибка отправки транзакции на реплику %s: %v", addr, err)
			conn.Close()
			delete(e.replicaConns, addr)
		}
	}
}

func (e *Engine) handleCommand(cmd Command) {
	switch cmd.action {
	case "insert":
		e.handleInsert(cmd)
	case "replace":
		e.handleReplace(cmd)
	case "delete":
		e.handleDelete(cmd)
	case "checkpoint":
		e.handleCheckpoint(cmd)
	case "search":
		e.handleSearch(cmd)
	default:
		cmd.result <- errors.New("неизвестная команда: " + cmd.action)
	}
	if e.leader && cmd.action != "search" && cmd.action != "checkpoint" {
		txn := Transaction{
			Action:  cmd.action,
			Name:    e.name,
			LSN:     e.lsn,
			Feature: cmd.feature,
		}
		e.broadcastTransaction(&txn)
	}
}

func (e *Engine) handleInsert(cmd Command) {
	if !e.leader {
		cmd.result <- errors.New("только лидер может создавать новые транзакции")
		return
	}
	e.lsn++
	e.lsn++
	txn := Transaction{
		Action:  "insert",
		Name:    e.name,
		LSN:     e.lsn,
		Feature: cmd.feature,
	}
	// Логирование транзакции
	if err := e.logTransaction(&txn); err != nil {
		cmd.result <- err
		return
	}
	// Обновление данных в памяти
	idStr, ok := cmd.feature.ID.(string)
	if !ok {
		cmd.result <- errors.New("ID объекта должен быть строкой")
		return
	}
	e.data[idStr] = cmd.feature
	minX, minY, maxX, maxY := getBoundingBox(cmd.feature.Geometry)
	e.spatialIdx.Insert([2]float64{minX, minY}, [2]float64{maxX, maxY}, cmd.feature)
	cmd.result <- nil
}

func (e *Engine) handleReplace(cmd Command) {
	if !e.leader {
		cmd.result <- errors.New("только лидер может создавать новые транзакции")
		return
	}
	e.lsn++
	txn := Transaction{
		Action:  "replace",
		Name:    e.name,
		LSN:     e.lsn,
		Feature: cmd.feature,
	}
	// Логирование транзакции
	if err := e.logTransaction(&txn); err != nil {
		cmd.result <- err
		return
	}
	// Обновление данных в памяти
	idStr, ok := cmd.feature.ID.(string)
	if !ok {
		cmd.result <- errors.New("ID объекта должен быть строкой")
		return
	}
	// Удаляем старый объект из индекса
	oldFeature, exists := e.data[idStr]
	if exists {
		minX, minY, maxX, maxY := getBoundingBox(oldFeature.Geometry)
		e.spatialIdx.Delete([2]float64{minX, minY}, [2]float64{maxX, maxY}, oldFeature)
	}
	e.data[idStr] = cmd.feature
	// Добавляем новый объект в индекс
	minX, minY, maxX, maxY := getBoundingBox(cmd.feature.Geometry)
	e.spatialIdx.Insert([2]float64{minX, minY}, [2]float64{maxX, maxY}, cmd.feature)
	cmd.result <- nil
}

func (e *Engine) handleDelete(cmd Command) {
	if !e.leader {
		cmd.result <- errors.New("только лидер может создавать новые транзакции")
		return
	}
	e.lsn++
	txn := Transaction{
		Action:  "delete",
		Name:    e.name,
		LSN:     e.lsn,
		Feature: cmd.feature,
	}
	// Логирование транзакции
	if err := e.logTransaction(&txn); err != nil {
		cmd.result <- err
		return
	}
	// Удаление данных из памяти и индекса
	idStr, ok := cmd.feature.ID.(string)
	if !ok {
		cmd.result <- errors.New("ID объекта должен быть строкой")
		return
	}
	feature, exists := e.data[idStr]
	if !exists {
		cmd.result <- errors.New("объект не найден")
		return
	}
	minX, minY, maxX, maxY := getBoundingBox(feature.Geometry)
	e.spatialIdx.Delete([2]float64{minX, minY}, [2]float64{maxX, maxY}, feature)
	delete(e.data, idStr)
	cmd.result <- nil
}

func (e *Engine) handleCheckpoint(cmd Command) {
	if err := e.checkpoint(); err != nil {
		cmd.result <- err
		return
	}
	cmd.result <- nil
}

func (e *Engine) handleSearch(cmd Command) {
	var features []*geojson.Feature
	e.spatialIdx.Search(cmd.min, cmd.max, func(min, max [2]float64, data interface{}) bool {
		feature, ok := data.(*geojson.Feature)
		if ok {
			features = append(features, feature)
		}
		return true // Продолжить поиск
	})
	// Отправляем результаты обратно через канал
	cmd.searchResult <- SearchResult{Features: features, Error: nil}
}

var upgrader = websocket.Upgrader{}

func (s *Storage) setupReplicationHandler() {
	s.mux.HandleFunc("/"+s.name+"/replication", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Ошибка апгрейда соединения: %v", err)
			return
		}
		addr := conn.RemoteAddr().String()
		s.engine.replicaConns[addr] = conn

		// Слушаем входящие сообщения от реплики
		go func() {
			defer func() {
				conn.Close()
				delete(s.engine.replicaConns, addr)
			}()
			for {
				var txn Transaction
				err := conn.ReadJSON(&txn)
				if err != nil {
					log.Printf("Ошибка чтения транзакции от реплики %s: %v", addr, err)
					return
				}
				s.engine.applyTransaction(&txn)
			}
		}()
	})
}

func (e *Engine) logTransaction(txn *Transaction) error {
	file, err := os.OpenFile("transactions.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(txn)
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, '\n'))
	return err
}

func (e *Engine) checkpoint() error {
	file, err := os.Create("checkpoint.json")
	if err != nil {
		return err
	}
	defer file.Close()

	checkpoint := Checkpoint{
		Data:   e.data,
		VClock: e.vclock,
	}

	encoder := json.NewEncoder(file)
	err = encoder.Encode(&checkpoint)
	if err != nil {
		return err
	}

	// Очистка журнала транзакций
	err = os.Remove("transactions.log")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (e *Engine) loadCheckpoint() error {
	file, err := os.Open("checkpoint.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Чекпоинт не существует
		}
		return err
	}
	defer file.Close()

	var checkpoint Checkpoint

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&checkpoint)
	if err != nil {
		return err
	}

	e.data = checkpoint.Data
	e.vclock = checkpoint.VClock
	for _, feature := range e.data {
		minX, minY, maxX, maxY := getBoundingBox(feature.Geometry)
		e.spatialIdx.Insert([2]float64{minX, minY}, [2]float64{maxX, maxY}, feature)
	}

	return nil
}

func (e *Engine) replayTransactions() error {
	file, err := os.Open("transactions.log")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Журнал транзакций пуст
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var txn Transaction
		err := json.Unmarshal(scanner.Bytes(), &txn)
		if err != nil {
			return err
		}
		e.applyTransaction(&txn)
	}
	return scanner.Err()
}

// Функция для получения bounding box из геометрии
func getBoundingBox(geometry orb.Geometry) (minX, minY, maxX, maxY float64) {
	bound := geometry.Bound()
	minX = bound.Min[0]
	minY = bound.Min[1]
	maxX = bound.Max[0]
	maxY = bound.Max[1]
	return
}

type Router struct {
	mux   *http.ServeMux
	nodes [][]string
	stop  chan struct{}
}

func NewRouter(mux *http.ServeMux, nodes [][]string) *Router {
	r := &Router{mux: mux, nodes: nodes, stop: make(chan struct{})}

	mux.Handle("/", http.FileServer(http.Dir("../front/dist")))
	mux.Handle("/select", http.RedirectHandler("/storage/select", http.StatusTemporaryRedirect))
	mux.Handle("/insert", http.RedirectHandler("/storage/insert", http.StatusTemporaryRedirect))
	mux.Handle("/replace", http.RedirectHandler("/storage/replace", http.StatusTemporaryRedirect))
	mux.Handle("/delete", http.RedirectHandler("/storage/delete", http.StatusTemporaryRedirect))
	mux.Handle("/checkpoint", http.RedirectHandler("/storage/checkpoint", http.StatusTemporaryRedirect))
	return r
}

func (r *Router) Run() {
	r.stop = make(chan struct{})
	go func() {
		<-r.stop
	}()

	log.Println("Router запущен")
}

func (r *Router) Stop() {
	if r.stop != nil {
		close(r.stop)
		log.Println("Router остановлен")
	}
}

type Storage struct {
	mux          *http.ServeMux
	name         string
	engine       *Engine
	stop         chan struct{}
	requestCount int
	requestLimit int
	replicas     []string
}

func NewStorage(mux *http.ServeMux, name string, replicas []string, leader bool) *Storage {
	ctx, cancel := context.WithCancel(context.Background())
	engine := &Engine{
		data:         make(map[string]*geojson.Feature),
		spatialIdx:   &rtree.RTree{},
		lsn:          0,
		name:         name,
		ctx:          ctx,
		cancel:       cancel,
		commands:     make(chan Command),
		vclock:       make(map[string]uint64),
		replicas:     replicas,
		leader:       leader,
		replicaConns: make(map[string]*websocket.Conn),
	}

	s := &Storage{
		mux:          mux,
		name:         name,
		engine:       engine,
		requestLimit: 3,        // Пороговое значение
		replicas:     replicas, // Список реплик
	}
	s.engine.Run()
	s.setupReplicationHandler()

	mux.HandleFunc("/"+name+"/select", func(w http.ResponseWriter, r *http.Request) {
		s.requestCount++

		// Проверяем, не достигнут ли порог
		if s.requestCount > s.requestLimit {
			// Сбрасываем счётчик
			s.requestCount = 0

			// Получаем адрес реплики для редиректа
			redirected := false
			for _, replicaAddr := range s.replicas {
				if replicaAddr != s.name {
					// Проверяем, не было ли уже редиректа
					if r.Header.Get("X-Redirected") == "true" {
						http.Error(w, "Too many redirects", http.StatusInternalServerError)
						return
					}
					// Добавляем заголовок
					r.Header.Set("X-Redirected", "true")
					http.Redirect(w, r, "http://"+replicaAddr+"/"+name+"/select"+r.URL.RawQuery, http.StatusTemporaryRedirect)
					redirected = true
					break
				}
			}
			if redirected {
				return
			}
		}
		minX, err := strconv.ParseFloat(r.URL.Query().Get("minX"), 64)
		if err != nil {
			http.Error(w, "Invalid minX parameter", http.StatusBadRequest)
			return
		}
		minY, err := strconv.ParseFloat(r.URL.Query().Get("minY"), 64)
		if err != nil {
			http.Error(w, "Invalid minY parameter", http.StatusBadRequest)
			return
		}
		maxX, err := strconv.ParseFloat(r.URL.Query().Get("maxX"), 64)
		if err != nil {
			http.Error(w, "Invalid maxX parameter", http.StatusBadRequest)
			return
		}
		maxY, err := strconv.ParseFloat(r.URL.Query().Get("maxY"), 64)
		if err != nil {
			http.Error(w, "Invalid maxY parameter", http.StatusBadRequest)
			return
		}

		cmd := Command{
			action:       "search",
			min:          [2]float64{minX, minY},
			max:          [2]float64{maxX, maxY},
			searchResult: make(chan SearchResult),
		}
		s.engine.commands <- cmd
		result := <-cmd.searchResult
		if result.Error != nil {
			http.Error(w, result.Error.Error(), http.StatusInternalServerError)
			return
		}
		fc := geojson.NewFeatureCollection()
		fc.Features = result.Features

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		s.requestCount--
	})

	mux.HandleFunc("/"+name+"/insert", func(w http.ResponseWriter, r *http.Request) {
		var feature geojson.Feature
		if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cmd := Command{
			action:  "insert",
			feature: &feature,
			result:  make(chan error),
		}
		s.engine.commands <- cmd
		if err := <-cmd.result; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/"+name+"/replace", func(w http.ResponseWriter, r *http.Request) {
		var feature geojson.Feature
		if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cmd := Command{
			action:  "replace",
			feature: &feature,
			result:  make(chan error),
		}
		s.engine.commands <- cmd
		if err := <-cmd.result; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/"+name+"/delete", func(w http.ResponseWriter, r *http.Request) {
		var feature geojson.Feature
		if err := json.NewDecoder(r.Body).Decode(&feature); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cmd := Command{
			action:  "delete",
			feature: &feature,
			result:  make(chan error),
		}
		s.engine.commands <- cmd
		if err := <-cmd.result; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/"+name+"/checkpoint", func(w http.ResponseWriter, r *http.Request) {
		cmd := Command{
			action: "checkpoint",
			result: make(chan error),
		}
		s.engine.commands <- cmd
		if err := <-cmd.result; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return s
}

func (s *Storage) Run() {
	s.stop = make(chan struct{})
	// Сервис Engine уже запущен в конструкторе
	log.Printf("Storage '%s' запущен", s.name)
}

func (s *Storage) Stop() {
	if s.engine != nil {
		s.engine.cancel()
	}
	if s.stop != nil {
		close(s.stop)
		log.Printf("Storage '%s' остановлен", s.name)
	}
}

func main() {
	storage1Mux := http.NewServeMux()
	storage1 := NewStorage(storage1Mux, "storage1", []string{"127.0.0.1:8081", "127.0.0.1:8082"}, true)

	storage2Mux := http.NewServeMux()
	storage2 := NewStorage(storage2Mux, "storage2", []string{"127.0.0.1:8080", "127.0.0.1:8082"}, false)

	storage3Mux := http.NewServeMux()
	storage3 := NewStorage(storage3Mux, "storage3", []string{"127.0.0.1:8080", "127.0.0.1:8081"}, false)
	go runStorage(storage1, storage1Mux, ":8080")
	go runStorage(storage2, storage2Mux, ":8081")
	go runStorage(storage3, storage3Mux, ":8082")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	storage1.Stop()
	storage2.Stop()
	storage3.Stop()
}

func runStorage(s *Storage, mux *http.ServeMux, addr string) {
	s.Run()
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	log.Printf("Storage '%s' is listening on %s", s.name, addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
