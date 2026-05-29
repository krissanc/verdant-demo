package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

var (
	db     *sql.DB
	dbReady atomic.Bool
	dbOnce  sync.Once
	tmpl   *template.Template
)

func main() {
	var err error
	tmpl, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	// Connect to DB in background — server starts immediately regardless.
	go connectDB()

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/join", handleJoin)
	mux.HandleFunc("/count", handleCount)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	log.Printf("verdant starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func connectDB() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Println("DATABASE_URL not set — running without database (set it and redeploy)")
		return
	}

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("db open error: %v", err)
		return
	}

	for i := 0; i < 30; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("db not ready, retry %d/30: %v", i+1, err)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Printf("db connection failed after retries: %v", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS waitlist (
		id         SERIAL PRIMARY KEY,
		email      TEXT UNIQUE NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		log.Printf("failed to create table: %v", err)
		return
	}

	dbReady.Store(true)
	log.Println("database connected and ready")
}

func signupCount() int {
	if !dbReady.Load() {
		return 0
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM waitlist").Scan(&n)
	return n
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := map[string]interface{}{
		"Count":   signupCount(),
		"DBReady": dbReady.Load(),
	}
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "template error", 500)
		log.Printf("template error: %v", err)
	}
}

func handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !dbReady.Load() {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<p class="mt-3 text-amber-400 text-sm">Database is warming up — try again in a moment.</p>`)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" || !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<p class="mt-3 text-red-400 text-sm">Please enter a valid email address.</p>`)
		return
	}

	_, err := db.Exec(
		"INSERT INTO waitlist (email) VALUES ($1) ON CONFLICT (email) DO NOTHING",
		email,
	)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<p class="mt-3 text-red-400 text-sm">Something went wrong. Try again.</p>`)
		log.Printf("insert error: %v", err)
		return
	}

	n := signupCount()
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="mt-4 py-4 px-6 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-center">
  <p class="text-emerald-300 font-semibold text-lg">You're on the list! 🎉</p>
  <p class="text-slate-400 text-sm mt-1">You're one of <strong class="text-white">%d</strong> people waiting for early access.</p>
</div>`, n)
}

func handleCount(w http.ResponseWriter, r *http.Request) {
	n := signupCount()
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w,
		`<span hx-get="/count" hx-trigger="every 8s" hx-swap="outerHTML" class="font-bold text-emerald-400">%d</span>`,
		n,
	)
}
