package main

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

type Plot struct {
	ID        int
	Number    string
	Area      float64
	Price     float64
	Status    string
	SvgPoints string
}

var db *pgxpool.Pool

var store = sessions.NewCookieStore([]byte("very-secret-key-change-later"))

func main() {
	var err error
	db, err = pgxpool.New(context.Background(),
		"postgres://postgres:postgres@localhost:5432/sakura")
	if err != nil {
		log.Fatal("Не удалось подключиться к БД:", err)
	}
	defer db.Close()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("POST /plots/{id}/reserve", reserveHandler)

	http.HandleFunc("GET /register", registerPageHandler)
	http.HandleFunc("POST /register", registerHandler)

	http.HandleFunc("GET /login", loginPageHandler)
	http.HandleFunc("POST /login", loginHandler)

	http.HandleFunc("GET /logout", logoutHandler)

	// Мои участки
	http.HandleFunc("GET /dashboard", requireAuth(dashboardHandler))

	http.HandleFunc("GET /admin/plots", requireAdmin(adminPlotsListHandler))
	http.HandleFunc("GET /admin/plots/new", requireAdmin(adminPlotNewPageHandler))
	http.HandleFunc("POST /admin/plots/new", requireAdmin(adminPlotCreateHandler))
	http.HandleFunc("GET /admin/plots/{id}/edit", requireAdmin(adminPlotEditPageHandler))
	http.HandleFunc("POST /admin/plots/{id}/edit", requireAdmin(adminPlotUpdateHandler))

	log.Println("Сервер запущен на http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// начальная страница
func indexHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	rows, err := db.Query(context.Background(),
		"Select id, number, area, price, status, svg_points From plots ORDER by id")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	defer rows.Close()

	var plots []Plot
	for rows.Next() {
		var p Plot
		err := rows.Scan(&p.ID, &p.Number, &p.Area, &p.Price, &p.Status, &p.SvgPoints)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		plots = append(plots, p)
	}

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	err = tmpl.Execute(w, map[string]interface{}{
		"Plots":    plots,
		"LoggedIn": userID != 0,
		"UserID":   userID,
	})
	if err != nil {
		log.Println("Ошибка рендеринга шаблона:", err)
	}
}

// страница резервирования
func reserveHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == 0 {
		http.Error(w, "Нужно войти в систему, чтобы бронировать участки", 401)
		return
	}
	id := r.PathValue("id")
	ctx := context.Background()

	_, err := db.Exec(ctx,
		"UPDATE plots SET status = 'reserved' , reserved_by = $1 WHERE id = $2 and status = 'available'", userID, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return

	}

	rows, err := db.Query(ctx,
		"SELECT id, number, area, price, status, svg_points FROM plots ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	defer rows.Close()

	var plots []Plot
	for rows.Next() {
		var p Plot
		err := rows.Scan(&p.ID, &p.Number, &p.Area, &p.Price, &p.Status, &p.SvgPoints)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		plots = append(plots, p)
	}
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	err = tmpl.ExecuteTemplate(w, "plots-inner", map[string]interface{}{"Plots": plots})
	if err != nil {
		log.Println("Ошибка рендеринга шаблона;", err)
	}
}

func registerPageHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/register.html"))
	tmpl.Execute(w, nil)
}

// регистрация пользователей
func registerHandler(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	_, err = db.Exec(context.Background(),
		"iNSERT iNTO USERS (email, password_hash) Values ($1,$2)", email, string(hash))
	if err != nil {
		http.Error(w, "Такой email  уже зарегестрирован", 400)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, nil)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	var userId int
	var passwordHash string

	err := db.QueryRow(context.Background(),
		"Select id, password_hash from users where email = $1", email).
		Scan(&userId, &passwordHash)
	if err != nil {
		http.Error(w, "Неверный email или пароль", 401)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if err != nil {
		http.Error(w, "Неверный email или пароль", 401)
		return
	}

	session, _ := store.Get(r, "session")
	session.Values["user_id"] = userId
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusFound)
}

func getUserID(r *http.Request) int {
	session, _ := store.Get(r, "session")
	userID, ok := session.Values["user_id"].(int)
	if !ok {
		return 0
	}
	return userID
}

// проверка пользователя на админскую роль

func isAdmin(r *http.Request) bool {
	userID := getUserID(r)
	if userID == 0 {
		return false
	}

	var role string
	err := db.QueryRow(context.Background(),
		"Select role From users where id =$1", userID).Scan(&role)
	if err != nil {
		return false
	}
	return role == "admin"
}

// разлогиниться
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	session.Values["user_id"] = nil
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusFound)
}

// Middleware авторизации
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// Middleware  для проверки админских прав

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r) {
			http.Error(w, "Доступ только для администраторов", 403)
			return
		}
		next(w, r)
	}
}

// Обработчик личного кабинета

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	rows, err := db.Query(context.Background(),
		"Select id, number , area, price, status, svg_points From plots where reserved_by = $1 ORDER by id",
		userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var plots []Plot
	for rows.Next() {
		var p Plot
		err := rows.Scan(&p.ID, &p.Number, &p.Area, &p.Price, &p.Status, &p.SvgPoints)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		plots = append(plots, p)
	}

	tmpl := template.Must(template.ParseFiles("templates/dashboard.html"))

	err = tmpl.Execute(w, map[string]interface{}{"Plots": plots})
	if err != nil {
		log.Println("Ошибка рендеринга шаблона:", err)
	}
}

// админская панель. список участков

func adminPlotsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(),
		"Select id, number,area,price,status,svg_points from plots order by id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	var plots []Plot

	for rows.Next() {
		var p Plot
		err := rows.Scan(
			&p.ID,
			&p.Number,
			&p.Area,
			&p.Price,
			&p.Status,
			&p.SvgPoints,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		plots = append(plots, p)
	}

	tmpl, err := template.ParseFiles("templates/admin_plots.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, map[string]interface{}{"Plots": plots}); err != nil {
		log.Println("Ошибка рендеринга шаблона:", err)
	}
}

// страничка для созания нового участка

func adminPlotNewPageHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/admin_plot_form.html"))
	tmpl.Execute(w, map[string]interface{}{"Plot": nil})
}

func adminPlotCreateHandler(w http.ResponseWriter, r *http.Request) {
	number := r.FormValue("number")
	area := r.FormValue("area")
	price := r.FormValue("price")
	status := r.FormValue("status")
	svgPoints := r.FormValue("svg_points")

	_, err := db.Exec(context.Background(),
		"Insert into plot (number, area,price,status,svg_points) VALUES($1, $2, $3, $4, $5)",
		number, area, price, status, svgPoints)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/plots", http.StatusFound)
}

// изменить участок

func adminPlotEditPageHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var p Plot
	err := db.QueryRow(context.Background(),
		"Select id, number,area,price,status, svg_points from plots where id = $1", id).Scan(&p.ID, &p.Number, &p.Area, &p.Price, &p.Status, &p.SvgPoints)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tmpl := template.Must(template.ParseFiles("templates/admin_plot_form.html"))
	tmpl.Execute(w, map[string]interface{}{"Plots": p})
}

// обновить участок

func adminPlotUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	number := r.FormValue("number")
	area := r.FormValue("area")
	price := r.FormValue("price")
	status := r.FormValue("status")
	svgPoints := r.FormValue("svg_points")

	_, err := db.Exec(context.Background(),
		"Update plots sen number = $1, area = $2, price =$3, status = $4, svg_points =$5 where id = $6",
		number, area, price, status, svgPoints, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/plots", http.StatusFound)
}
