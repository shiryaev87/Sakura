# План проекта: сайт бронирования земельных участков

**Стек:** Go + html/template + htmx + PostgreSQL + Redis + Docker
**Уровень:** для новичка в Go — план построен так, чтобы после каждого шага был видимый результат в браузере.

---

## Общая архитектура

```
Браузер
  │
  │ HTTP-запросы (обычные и htmx)
  ▼
Go-сервер (net/http)
  │  ├── html/template   → рендерит HTML прямо на сервере
  │  ├── обработчики      → GET/POST хендлеры
  │  └── middleware       → проверка авторизации (сессии)
  │
  ├──► PostgreSQL (Docker) → участки, брони, пользователи
  └──► Redis (Docker)      → временная блокировка участка при бронировании
```

Один язык (Go), один рантайм, минимум зависимостей. Node.js/React не используются — интерактивность даёт htmx (один JS-файл, без сборщиков).

---

## Общий план (12 этапов)

| № | Этап | Результат, который увидите |
|---|------|------------------------------|
| 0 | Подготовка окружения | Установлены Go, Docker, SQL-клиент |
| 1 | Hello World на Go | Страница в браузере на `localhost:8080` |
| 2 | Postgres в Docker | Таблица `plots` с тестовыми данными |
| 3 | Go читает из БД | Список участков рендерится из реальной БД |
| 4 | Подключение htmx | Кнопка "Забронировать" работает без перезагрузки страницы |
| 5 | Визуальная схема (SVG) | Участки — цветные кликабельные полигоны |
| 6 | Логика бронирования | Клик реально меняет статус в БД (ядро MVP) |
| 7 | Redis-блокировка | Защита от двойного бронирования одного участка |
| 8 | Пользователи и вход | Таблица `users`, регистрация, логин, сессии |
| 9 | Личный кабинет клиента | Страница "Мои брони" только для залогиненных |
| 10 | Простая админка | Агент может добавлять/редактировать участки |
| 11 | Документы и уведомления | PDF-договор, email/SMS о статусе брони |
| 12 | Деплой в интернет | Сайт доступен по домену из любой точки мира |

Этапы 0–6 — это рабочий MVP, который стоит пройти без остановок. Дальше можно делать по своей скорости.

---

## Детальный план

### Этап 0. Подготовка окружения

**Установить:**
- Go (golang.org/dl) — версия 1.22+
- Docker Desktop (docker.com)
- SQL-клиент: DBeaver (бесплатный, кроссплатформенный) или расширение "PostgreSQL" в VS Code
- Редактор кода: VS Code + расширение "Go" (от команды Google)

**Проверка:**
```bash
go version
docker --version
```
Обе команды должны вывести версию без ошибок.

**Основы Go, которые понадобятся перед стартом (по 10-15 минут на каждую тему, можно гуглить/смотреть по ходу дела):**
- Синтаксис: переменные, функции, `if/for`
- Структуры (`struct`) — аналог классов без методов внутри
- Указатели (`*`, `&`) — базовое понимание, что это адрес в памяти
- Обработка ошибок через `if err != nil` (в Go нет try/catch, это нормально и обычно)
- Пакеты (`package`, `import`)

Не нужно проходить полный курс — достаточно общего представления, разберётесь по ходу написания кода.

---

### Этап 1. Hello World на Go

**Цель:** увидеть, что Go-сервер отвечает в браузере HTML-страницей.

**Файлы:**
```
land-booking/
├── go.mod
└── main.go
```

**Команды:**
```bash
mkdir land-booking && cd land-booking
go mod init land-booking
```

**main.go:**
```go
package main

import (
	"html/template"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.New("index").Parse(`
			<html>
			<head><title>Земельные участки</title></head>
			<body><h1>Проект запущен</h1></body>
			</html>
		`))
		tmpl.Execute(w, nil)
	})

	log.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

**Запуск:**
```bash
go run main.go
```

Открыть `http://localhost:8080` в браузере.

**Проверка успеха:** видите заголовок "Проект запущен" на странице.

---

### Этап 2. PostgreSQL и Redis в Docker

**Цель:** поднять базу данных без ручной установки.

**docker-compose.yml** (в корне проекта, рядом с main.go):
```yaml
services:
  postgres:
    image: postgres:16
    container_name: land_postgres
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: land_booking
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7
    container_name: land_redis
    ports:
      - "6379:6379"

volumes:
  pgdata:
```

**Запуск:**
```bash
docker-compose up -d
docker ps    # проверка, что оба контейнера запущены
```

**Создать таблицу через SQL-клиент** (подключение: host `localhost`, порт `5432`, юзер `postgres`, пароль `postgres`, база `land_booking`):

```sql
CREATE TABLE plots (
    id SERIAL PRIMARY KEY,
    number TEXT NOT NULL,
    area_sotka NUMERIC NOT NULL,
    price NUMERIC NOT NULL,
    status TEXT NOT NULL DEFAULT 'available', -- available | reserved | sold
    created_at TIMESTAMP DEFAULT NOW()
);

INSERT INTO plots (number, area_sotka, price, status) VALUES
('1', 6, 500000, 'available'),
('2', 8, 650000, 'available'),
('3', 10, 800000, 'sold'),
('4', 6, 500000, 'reserved'),
('5', 12, 950000, 'available');
```

**Проверка успеха:** в DBeaver видите 5 строк в таблице `plots`.

---

### Этап 3. Go читает из базы и отдаёт HTML

**Цель:** соединить Go и Postgres, вывести реальные данные на страницу.

**Установить драйвер БД:**
```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
```

**Структура проекта расширяется:**
```
land-booking/
├── go.mod
├── main.go
├── docker-compose.yml
└── templates/
    └── index.html
```

**templates/index.html:**
```html
<html>
<head><title>Земельные участки</title></head>
<body>
  <h1>Каталог участков</h1>
  {{range .Plots}}
    <div style="border:1px solid #ccc; padding:10px; margin:10px;">
      Участок №{{.Number}} — {{.AreaSotka}} соток — {{.Price}} руб — статус: {{.Status}}
    </div>
  {{end}}
</body>
</html>
```

**main.go (ключевые части):**
```go
package main

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Plot struct {
	ID        int
	Number    string
	AreaSotka float64
	Price     float64
	Status    string
}

var db *pgxpool.Pool

func main() {
	var err error
	db, err = pgxpool.New(context.Background(),
		"postgres://postgres:postgres@localhost:5432/land_booking")
	if err != nil {
		log.Fatal("Не удалось подключиться к БД:", err)
	}
	defer db.Close()

	http.HandleFunc("/", indexHandler)

	log.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(context.Background(),
		"SELECT id, number, area_sotka, price, status FROM plots ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var plots []Plot
	for rows.Next() {
		var p Plot
		rows.Scan(&p.ID, &p.Number, &p.AreaSotka, &p.Price, &p.Status)
		plots = append(plots, p)
	}

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, map[string]interface{}{"Plots": plots})
}
```

**Запуск:** `go run main.go` (Postgres из Docker должен быть уже поднят).

**Проверка успеха:** на `localhost:8080` видите список из 5 участков — данные из реальной БД, не захардкоженные.

---

### Этап 4. Подключение htmx для интерактивности

**Цель:** добавить кнопку, которая меняет данные без перезагрузки страницы.

**Изменить templates/index.html** — добавить htmx-скрипт и кнопку:
```html
<html>
<head>
  <title>Земельные участки</title>
  <script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head>
<body>
  <h1>Каталог участков</h1>
  {{range .Plots}}
    {{template "plot-card" .}}
  {{end}}
</body>
</html>

{{define "plot-card"}}
<div id="plot-{{.ID}}" style="border:1px solid #ccc; padding:10px; margin:10px;">
  Участок №{{.Number}} — {{.AreaSotka}} соток — {{.Price}} руб — статус: {{.Status}}
  {{if eq .Status "available"}}
    <button hx-post="/plots/{{.ID}}/reserve" hx-target="#plot-{{.ID}}" hx-swap="outerHTML">
      Забронировать
    </button>
  {{end}}
</div>
{{end}}
```

**Добавить хендлер в main.go:**
```go
func reserveHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	_, err := db.Exec(context.Background(),
		"UPDATE plots SET status = 'reserved' WHERE id = $1 AND status = 'available'", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var p Plot
	db.QueryRow(context.Background(),
		"SELECT id, number, area_sotka, price, status FROM plots WHERE id = $1", id).
		Scan(&p.ID, &p.Number, &p.AreaSotka, &p.Price, &p.Status)

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.ExecuteTemplate(w, "plot-card", p)
}

// в main():
http.HandleFunc("POST /plots/{id}/reserve", reserveHandler)
```

**Проверка успеха:** кликаете "Забронировать" — карточка сама, без перезагрузки страницы, меняется на статус "reserved", кнопка исчезает. Обновляете страницу (F5) — статус сохранился, значит реально записался в БД.

**Это первая полностью рабочая версия MVP — стоит остановиться и порадоваться.**

---

### Этап 5. Визуальная схема участков (SVG)

**Цель:** заменить список карточек на кликабельную схему.

Простой пример SVG с полигонами-прямоугольниками вместо `{{range .Plots}}` со списком:

```html
<svg viewBox="0 0 400 300" width="800" height="600">
  {{range .Plots}}
  <polygon points="{{.SvgPoints}}"
           fill="{{if eq .Status "available"}}#4ade80{{else if eq .Status "reserved"}}#fbbf24{{else}}#f87171{{end}}"
           stroke="#333"
           hx-post="/plots/{{.ID}}/reserve"
           hx-target="closest svg"
           hx-swap="outerHTML" />
  {{end}}
</svg>
```

Координаты (`SvgPoints`) можно на старте прописать вручную в БД или коде для 5-10 тестовых участков (просто прямоугольники сетки), позже — обвести реальный генплан в Inkscape.

**Проверка успеха:** видите сетку цветных прямоугольников, клик меняет цвет.

---

### Этап 6. Redis — защита от двойного бронирования

**Цель:** не дать двум людям одновременно забронировать один участок.

**Установить клиент:**
```bash
go get github.com/redis/go-redis/v9
```

**Логика:** перед `UPDATE` в Postgres — пробуем поставить блокировку в Redis через `SETNX` с TTL 15 минут. Если блокировка не удалась — участок уже кто-то бронирует, показываем ошибку.

```go
ok, err := redisClient.SetNX(ctx, "lock:plot:"+id, "1", 15*time.Minute).Result()
if !ok {
    http.Error(w, "Участок уже бронируется другим пользователем", 409)
    return
}
```

Атомарный `UPDATE ... WHERE status = 'available'` из Этапа 4 уже частично защищает от гонок на уровне БД — Redis добавляет более быструю и явную блокировку на уровне UX.

---

### Этап 7. Пользователи и авторизация

**Цель:** регистрация, вход, сессии.

**Установить:**
```bash
go get github.com/gorilla/sessions
go get golang.org/x/crypto/bcrypt
```

**Таблица:**
```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);

ALTER TABLE plots ADD COLUMN reserved_by INTEGER REFERENCES users(id);
```

**Регистрация — хеш пароля:**
```go
hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
```

**Вход — проверка и создание сессии:**
```go
err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(inputPassword))
if err == nil {
    session, _ := store.Get(r, "session")
    session.Values["user_id"] = user.ID
    session.Save(r, w)
}
```

**Middleware для защиты страниц:**
```go
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        session, _ := store.Get(r, "session")
        if session.Values["user_id"] == nil {
            http.Redirect(w, r, "/login", http.StatusFound)
            return
        }
        next(w, r)
    }
}
```

**Проверка успеха:** регистрируетесь, входите, при попытке зайти на `/dashboard` без входа — редирект на `/login`.

---

### Этап 8. Личный кабинет клиента

**Цель:** страница "Мои брони", видна только своя.

**Хендлер:**
```go
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	userID := session.Values["user_id"]

	rows, _ := db.Query(context.Background(),
		"SELECT id, number, price, status FROM plots WHERE reserved_by = $1", userID)
	// ... собрать в слайс, отрендерить шаблон dashboard.html
}
```

Регистрируем с защитой:
```go
http.HandleFunc("/dashboard", requireAuth(dashboardHandler))
```

**Проверка успеха:** видите список только своих забронированных участков.

---

### Этап 9. Простая админка для агента

**Цель:** агент может добавлять и редактировать участки без прямого доступа к БД.

- Отдельная роль в таблице `users` (`role: 'admin' | 'client'`)
- Страницы `/admin/plots` (список), `/admin/plots/new` (форма создания), `/admin/plots/{id}/edit`
- Тот же принцип: `html/template` + htmx-формы + middleware, проверяющий `role == 'admin'`

На старте для 1-2 агентов можно продолжать работать через DBeaver — этот этап не критичен для первой версии.

---

### Этап 10. Документы и уведомления

- Генерация PDF-договора при подтверждении брони (библиотека `github.com/jung-kurt/gofpdf` или `github.com/johnfercher/maroto`)
- Email через SMTP (стандартный пакет `net/smtp` или сторонний `gopkg.in/gomail.v2`)
- SMS — через API любого российского SMS-провайдера (SMS.ru, SMSC.ru — обычный HTTP-запрос из Go)

---

### Этап 11. Деплой в интернет

1. Написать `Dockerfile` для Go-приложения (multi-stage build)
2. Добавить свой сервис в `docker-compose.yml` рядом с Postgres/Redis
3. Взять VPS (Yandex Cloud / Hetzner / DigitalOcean)
4. `ssh` на сервер, поставить Docker, `git clone`, `docker-compose up -d --build`
5. Купить домен, направить A-запись на IP сервера
6. Настроить Nginx + Let's Encrypt (HTTPS)

---

## Рекомендации для новичка в Go

- **Не проходите Go "с нуля до конца" перед стартом** — учите синтаксис по ходу написания реального кода, так усваивается быстрее
- **Официальный тур** — [go.dev/tour](https://go.dev/tour) — 1-2 вечера, достаточно для базового синтаксиса
- **Не бойтесь `err != nil` через строку** — это нормальный стиль Go, не признак того, что что-то не так
- **Каждый этап — это отдельный маленький коммит в git.** Даже если не публикуете проект сразу, `git init` + `git commit` после каждого рабочего шага — чтобы всегда можно было откатиться
- **Не переходите к следующему этапу, пока текущий не работает видимо в браузере** — план построен именно так специально

---

## Что делать прямо сейчас

1. Выполнить Этап 0 (установка инструментов)
2. Написать и запустить Этап 1 (`main.go` с Hello World)
3. Написать в чат, когда увидите страницу в браузере — двигаемся к Этапу 2
