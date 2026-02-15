package handlers

import (
	"file-sharing/internal/hub"
	"file-sharing/internal/transfer"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWS управляет WebSocket соединениями
func HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	id := fmt.Sprintf("User-%s", r.RemoteAddr[len(r.RemoteAddr)-5:])

	hub.Mu.Lock()
	hub.Clients[id] = conn
	conn.WriteJSON(map[string]string{"type": "welcome", "id": id})
	hub.Mu.Unlock()
	hub.Broadcast()

	defer func() {
		hub.Mu.Lock()
		delete(hub.Clients, id)
		hub.Mu.Unlock()
		hub.Broadcast()
		conn.Close()
	}()

	for {
		var msg map[string]string
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		hub.Mu.Lock()
		if target, ok := hub.Clients[msg["to"]]; ok {
			msg["from"] = id
			target.WriteJSON(msg)
		}
		hub.Mu.Unlock()
	}
}

// HandleStream управляет передачей файлов
func HandleStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("to")
	t := transfer.GetOrCreate(id)

	// Создаем буфер 1МБ для ускорения передачи
	buf := make([]byte, 1024*1024)

	if r.Method == http.MethodPost {
		// Используем CopyBuffer вместо обычного Copy
		_, err := io.CopyBuffer(t.Pw, r.Body, buf)
		if err != nil {
			fmt.Println("Ошибка POST:", err)
		}
		t.Pw.Close()
		transfer.Delete(id)
	} else {
		//Указываем, что это скачивание файла
		w.Header().Set("Content-Disposition", "attachment; filename="+r.URL.Query().Get("name"))
		//Указываем точный размер (КРИТИЧНО для прогресс-бара)
		w.Header().Set("Content-Length", r.URL.Query().Get("size"))

		// Для скачивания тоже используем буфер 1МБ
		_, err := io.CopyBuffer(w, t.Pr, buf)
		if err == nil {
			hub.Mu.Lock()
			// 1. Уведомляем Отправителя (его ID берем из параметра from)
			senderID := r.URL.Query().Get("from")
			if conn, ok := hub.Clients[senderID]; ok {
				conn.WriteJSON(map[string]string{"type": "complete"})
			}
			// 2. Уведомляем Получателя (его ID берем из параметра to)
			receiverID := r.URL.Query().Get("to")
			if conn, ok := hub.Clients[receiverID]; ok {
				conn.WriteJSON(map[string]string{"type": "complete"})
			}
			hub.Mu.Unlock()
		}

		// Очищаем передачу после завершения
		transfer.Delete(id)
	}
}
