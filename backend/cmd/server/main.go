package main

import (
	"log"
	"net/http"

	"gitflame-codepilot/backend/internal/app"
)

func main() {
	cfg := app.LoadConfig()
	server := app.NewServer(cfg)

	log.Printf("GitFlame CodePilot backend listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server.Router()); err != nil {
		log.Fatal(err)
	}
}
