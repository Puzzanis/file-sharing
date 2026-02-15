package main

import (
	"file-sharing/internal/db"
	"fmt"
	"net/http"

	"file-sharing/internal/handlers"
)

func main() {
	db.InitDB() // Инициализация SQLite
	defer db.DB.Close()

	db.PrintStats()

	fs := http.FileServer(http.Dir("./ui"))
	http.Handle("/", fs)

	http.HandleFunc("/ws", handlers.HandleWS)
	http.HandleFunc("/stream", handlers.HandleStream)

	port := ":8080"
	fmt.Printf("🚀 Secure Server запущен: https://localhost%s\n", port)

	// Используем ListenAndServeTLS вместо обычного ListenAndServe
	err := http.ListenAndServeTLS(port, "cert.pem", "key.pem", nil)
	if err != nil {
		fmt.Printf("Критическая ошибка TLS: %v\n", err)
	}
}
