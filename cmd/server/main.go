package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"restaurant_network_pos/internal/app"
	"restaurant_network_pos/internal/db"
	"syscall"
	"time"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file, reading environment variables directly")
	}

	database, err := db.New()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	log.Println("connected to MS SQL Server")

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		log.Fatal("SESSION_SECRET is not set")
	}
	store := sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	router, cleanup := app.SetupRouter(database, store)
	srv.Handler = router
	srv.RegisterOnShutdown(cleanup)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
		if cerr := srv.Close(); cerr != nil {
			log.Printf("force close error: %v", cerr)
		}
	}
	if err := database.Close(); err != nil {
		log.Printf("database close error: %v", err)
	}
	log.Println("server stopped")
}
