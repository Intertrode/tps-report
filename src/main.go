package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	Port       string
}

func loadConfig() Config {
	cfg := Config{
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		Port:       os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	required := map[string]string{
		"DB_HOST":     cfg.DBHost,
		"DB_PORT":     cfg.DBPort,
		"DB_USER":     cfg.DBUser,
		"DB_PASSWORD": cfg.DBPassword,
		"DB_NAME":     cfg.DBName,
	}
	for name, val := range required {
		if val == "" {
			log.Fatalf("required environment variable %s is not set", name)
		}
	}

	return cfg
}

func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}

type Report struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func initDB(dsn string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := db.AutoMigrate(&Report{}); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	return db
}

type server struct {
	db *gorm.DB
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := s.db.DB()
	if err != nil {
		http.Error(w, "db handle error", http.StatusServiceUnavailable)
		return
	}
	if err := sqlDB.Ping(); err != nil {
		http.Error(w, "db unreachable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var rep Report
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	rep.ID = 0
	if err := s.db.Create(&rep).Error; err != nil {
		http.Error(w, "failed to create report", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

func (s *server) handleGetReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var rep Report
	err = s.db.First(&rep, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *server) handleListReports(w http.ResponseWriter, r *http.Request) {
	var reps []Report
	if err := s.db.Order("id").Find(&reps).Error; err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, reps)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	cfg := loadConfig()
	db := initDB(cfg.DSN())
	srv := &server{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.handleHealthz)
	mux.HandleFunc("POST /reports", srv.handleCreateReport)
	mux.HandleFunc("GET /reports/{id}", srv.handleGetReport)
	mux.HandleFunc("GET /reports", srv.handleListReports)

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
