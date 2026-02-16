package handlers

import (
	"file-sharing/internal/db"
	"file-sharing/internal/hub"
	"file-sharing/internal/transfer"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
	senderID := r.URL.Query().Get("from")
	fileName := r.URL.Query().Get("name")
	sizeStr := r.URL.Query().Get("size")

	// Получаем или создаем трубу
	t := transfer.GetOrCreate(id)
	buf := make([]byte, 1024*1024)

	if r.Method == http.MethodPost {
		// ОТПРАВИТЕЛЬ
		_, err := io.CopyBuffer(t.Pw, r.Body, buf)
		t.Pw.Close() // Обязательно закрываем, чтобы получатель узнал о конце файла
		if err != nil {
			fmt.Println("Ошибка POST:", err)
		}
	} else {
		// ПОЛУЧАТЕЛЬ
		w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
		w.Header().Set("Content-Length", sizeStr)
		w.Header().Set("Content-Type", "application/octet-stream")

		_, err := io.CopyBuffer(w, t.Pr, buf)

		// После того как CopyBuffer закончил работу (файл передан)
		if err == nil {
			// Логируем в БД
			fileSizeInt, _ := strconv.ParseInt(sizeStr, 10, 64)
			db.LogTransfer(senderID, id, fileName, fileSizeInt)

			// Уведомляем участников через WebSocket
			hub.Mu.Lock()
			if senderConn, ok := hub.Clients[senderID]; ok {
				senderConn.WriteJSON(map[string]string{"type": "complete"})
			}
			if receiverConn, ok := hub.Clients[id]; ok {
				receiverConn.WriteJSON(map[string]string{"type": "complete"})
			}
			hub.Mu.Unlock()
		}

		// Очищаем передачу ТОЛЬКО после того, как получатель закончил чтение
		transfer.Delete(id)
	}
}
