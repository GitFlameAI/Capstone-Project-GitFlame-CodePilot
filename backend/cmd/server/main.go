package main

import (
	"log"
	"net/http"

	"gitflame-codepilot/backend/internal/app"
)

func main() {
	cfg := app.LoadConfig()
	server, err := app.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	log.Printf("GitFlame CodePilot backend listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server.Router()); err != nil {
		log.Fatal(err)
	}
}
