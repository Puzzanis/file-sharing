package hub

import (
	"sync"

	"github.com/gorilla/websocket"
)

var (
	// Карта активных WebSocket-клиентов: ID пользователя -> его соединение
	Clients = make(map[string]*websocket.Conn)
	// Мьютекс для безопасного доступа к картам из разных потоков (горутин)
	Mu sync.Mutex
)

// broadcast рассылает актуальный список всех ID пользователей всем подключенным
func Broadcast() {
	var list []string
	for id := range Clients {
		list = append(list, id)
	}
	for _, c := range Clients {
		c.WriteJSON(map[string]interface{}{"type": "list", "users": list})
	}
}
