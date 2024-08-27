# Практика 1

![Saint-Petersburg](img/cover.png)

## ТЗ

Сделаем на Golang геоинформационную базу данных.
Архитектуру сделаем сразу по взрослому. А реализацию простой и наивной.
Для серализации данных используем `geojson` формат https://geojson.org.
Для идентификации `geojson` используем поле `ID`. Поле `ID` заполняется значением `uuid` на фронтенде.
Информацию об объектах просто сохраняем в `geo.db.json` файл.
Для работы с фронтендом реализовываем `http api`.

## Архитектура

![Architecture](img/practice1-arch.png)

## Более точная архитектура

![Architecture](img/practice1-arch-2.png)

## Стек технологий

- backend: golang
- frontend: готовый проект на openlayers

## Backend

Реализуем три части.
- Первая часть наносервис Router.
- Вторая часть наносервис Storage.
- Третья часть запуск и останов http.Server

Для обработки GeoJSON данных возьмем библиотеку `github.com/paulmach/orb/geojson`

## Наносервис

Скелет наносервиса:

```
struct Router {
  // поля
}

func NewRouter(аргументы) *Router { конструктор нано сервиса }

func (r *Router) Run() { }
func (r *Router) Stop() { }
```

Сделаем наносервис Router, который регистрирует в http.ServerMux
Сделаем наносервис Storage, который регистрирует в http.ServerMux

Конструктор структуры Router:
- `NewRouter(mux *http.ServeMux, nodes [][]string) *Router`
  - `mux`

Конструктор структуры Storage:
- `NewStorage(mux *http.ServeMux, name string, replicas []string, leader bool) *Storage`

Расположим в структурах методы обработчики `http api`.

Организовать наносервисы следующим образом.
- Сделаем глобальный объект `mux *http.ServerMux`
- Передадим объект и в конструктор `Router` и в конструктор `Storage`
- Зададим произвольное имя для `Storage`
- Запустим и `Storage` и `Router` в отдельных горутинах
- Передадим это имя в конструктор `Router`
- Запустим `http server` на `127.0.0.1:8080`
  - Передадим туда глобальный `mux`
- Запустим `http server` в отдельной горутине
- Запустим ожидание сигнала
  ```
  sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	slog.Info("got", "signal", sig)
	```
- После получения сигнала остановим сервисы в обратном порядке
  - `http.Server`
  - `Router`
  - `Storage`

## HTTP API

API возвращает:
  - 200 если всё ок
  - Иначе код ошибки и сообщение в body

### Router

HTTP endpoints:
- `GET /*`
  - Вернуть ресурсы фронтенда из директории `../front/dist/*`
- `GET /select`
  - Редиректить на Storage с кодом http.StatusTemporaryRedirect
- `POST /insert`
  - Редиректить на Storage с кодом http.StatusTemporaryRedirect
- `POST /replace`
  - Редиректить на Storage с кодом http.StatusTemporaryRedirect
- `POST /delete`
  - Редиректить на Storage с кодом http.StatusTemporaryRedirect

### Storage

HTTP endpoints:
- `GET {name}/select`
  - Вернуть `geojson` объекты в формате `feature collection`
- `POST {name}/insert`
  - Сохранить новый `geojson` объект из `body` запроса
- `POST {name}/replace`
  - Заменить сохранённый ранее `geojson` объект из `body` запроса
- `POST {name}/delete`
  - Удалить сохранённый ранее `geojson` объект `id` из `body` запроса

## Тестирование

Сделаем тест на вставку, замену и удаление данных.
Для тестов можно воспользоваться табличными тестами.
Для тестирования http запросов можно пользоваться объектами:
- `http.NewRequest(...)` - для запроса
- `httptest.NewRecorder(...)` — для записи ответа
