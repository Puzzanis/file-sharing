package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Настройка Upgrader: преобразует обычный HTTP-запрос в WebSocket-соединение
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }}

// Transfer структура для передачи файла между пользователями
type Transfer struct {
	pr *io.PipeReader // Сторона, которая читает (получатель)
	pw *io.PipeWriter // Сторона, которая пишет (отправитель)
}

var (
	// Карта активных WebSocket-клиентов: ID пользователя -> его соединение
	clients = make(map[string]*websocket.Conn)
	// Карта активных передач: ID получателя
	transfers = make(map[string]*Transfer)
	// Мьютекс для безопасного доступа к картам из разных потоков (горутин)
	mu sync.Mutex
)

func main() {
	// Регистрация маршрутов
	http.HandleFunc("/", handleHome)         // Главная страница с интерфейсом
	http.HandleFunc("/ws", handleWS)         // Сигнальный сервер (кто в сети, уведомления)
	http.HandleFunc("/stream", handleStream) // Канал для самой передачи байтов файла
	fmt.Println("🚀 Сервер запущен: http://localhost:8080")
	// Запуск сервера на порту 8080
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Ошибка старта:", err)
	}
}

// handleWS управляет списком пользователей и пересылкой сигналов (offer/accept)
func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, _ := upgrader.Upgrade(w, r, nil)
	// Генерируем ID пользователя на основе его сетевого порта (последние 5 символов)
	id := fmt.Sprintf("User-%s", r.RemoteAddr[len(r.RemoteAddr)-5:])
	mu.Lock()
	clients[id] = conn
	// Сразу сообщаем клиенту его собственный ID
	conn.WriteJSON(map[string]string{"type": "welcome", "id": id})
	broadcast()
	mu.Unlock()
	// Удаление клиента при отключении
	defer func() {
		mu.Lock()
		delete(clients, id)
		broadcast()
		mu.Unlock()
		conn.Close()
	}()
	// Цикл прослушивания входящих сообщений (сигналов) от клиента
	for {
		var msg map[string]string
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		mu.Lock()
		if target, ok := clients[msg["to"]]; ok {
			msg["from"] = id
			target.WriteJSON(msg)
		}
		mu.Unlock()
	}
}

// broadcast рассылает актуальный список всех ID пользователей всем подключенным
func broadcast() {
	var list []string
	for id := range clients {
		list = append(list, id)
	}
	for _, c := range clients {
		c.WriteJSON(map[string]interface{}{"type": "list", "users": list})
	}
}

// handleHome отдает HTML, CSS и JavaScript фронтенд приложения
func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Go File Share</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, sans-serif; background: #f0f2f5; display: flex; flex-direction: column; align-items: center; padding: 20px; }
        .card { background: white; padding: 20px; border-radius: 12px; width: 380px; box-shadow: 0 4px 10px rgba(0,0,0,0.1); margin-top: 15px; border: 1px solid #ddd; }
        .user { padding: 12px; border-bottom: 1px solid #f0f0f0; display: flex; justify-content: space-between; align-items: center; }
        .user:last-child { border-bottom: none; }
        #notif { display:none; border: 2px solid #28a745; background: #f1f8e9; }
        button { cursor: pointer; border: none; padding: 8px 15px; border-radius: 6px; background: #007bff; color: white; font-weight: bold; }
        button:hover { background: #0056b3; }
        .btn-ok { background: #28a745; margin-right: 10px; } 
        .btn-no { background: #dc3545; }
        #status { font-size: 14px; color: #555; margin-top: 15px; text-align: center; min-height: 20px; }
        .progress-container { width: 100%; background: #e0e0e0; border-radius: 10px; height: 12px; margin-top: 10px; display: none; overflow: hidden; }
        .progress-bar { width: 0%; height: 100%; background: #28a745; transition: width 0.1s linear; }
    </style>
</head>
<body>
<!-- Уведомление о входящем файле -->
<div id="notif" class="card">
    <strong id="notif-txt"></strong>
    <div style="margin-top:15px; display: flex; justify-content: center;">
        <button class="btn-ok" onclick="reply(true)">Принять</button>
        <button class="btn-no" onclick="reply(false)">Отмена</button>
    </div>
</div>
<!-- Основной интерфейс -->
<div class="card">
    <h3 style="margin-top:0">Люди в сети:</h3>
    <div id="list">Загрузка...</div>
    <input type="file" id="file-input" style="display:none">
    
    <div id="p-cont" class="progress-container">
        <div id="p-bar" class="progress-bar"></div>
    </div>
    <div id="status">Инициализация...</div>
</div>

<script>
    const ws = new WebSocket('ws://' + location.host + '/ws');
    let myId, currentOffer = null, fileToSend = null;

    // Функция для красивого вывода размера (Байты -> Кб/Мб/Гб)
    function formatBytes(bytes, decimals = 2) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const dm = decimals < 0 ? 0 : decimals;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
    }
	// Обновление полоски прогресса
    function updateProgress(percent) {
        const cont = document.getElementById('p-cont');
        const bar = document.getElementById('p-bar');
        cont.style.display = 'block';
        bar.style.width = percent + '%';
    }
	// Обработка сообщений от сервера по WebSocket
    ws.onmessage = (e) => {
        const d = JSON.parse(e.data);
        if(d.type === 'welcome') {
            myId = d.id;
            document.getElementById('status').innerText = "Ваш ID: " + myId;
        }
        
        if(d.type === 'list') {
            const listDiv = document.getElementById('list');
            listDiv.innerHTML = "";
            d.users.forEach(id => {
                const isMe = id === myId;
                listDiv.innerHTML += '<div class="user"><span>' + id + (isMe ? ' <strong>(Вы)</strong>' : '') + '</span>' + 
                    (!isMe ? '<button onclick="askSend(\''+id+'\')">Файл</button>' : '') + '</div>';
            });
        }

        if(d.type === 'offer') {
            currentOffer = d;
            document.getElementById('notif-txt').innerHTML = 
                "От: " + d.from + "<br>" +
                "Файл: <b>" + d.name + "</b><br>" +
                "Размер: <span style='color:#007bff'>" + formatBytes(parseInt(d.size)) + "</span>";
            
            document.getElementById('notif').style.display = 'block';
        }

        if(d.type === 'accept') {
            uploadFile(d.from); // Если получатель нажал Принять, начинаем POST-отправку
        }
    };

    function askSend(toId) {
        const input = document.getElementById('file-input');
        input.onchange = () => {
            if (input.files.length === 0) return;
            fileToSend = input.files[0];
            
            // Отправляем имя и размер через WebSocket
            ws.send(JSON.stringify({
                type: 'offer', 
                to: toId, 
                name: fileToSend.name, 
                size: fileToSend.size.toString() 
            }));
            document.getElementById('status').innerText = "Ждем ответа от " + toId + "...";
        };
        input.click();
    }

    function reply(ok) {
        document.getElementById('notif').style.display = 'none';
        if(ok && currentOffer) {
            ws.send(JSON.stringify({type: 'accept', to: currentOffer.from}));
            
            const url = "/stream?to=" + myId + "&name=" + encodeURIComponent(currentOffer.name) + "&size=" + currentOffer.size;
            const link = document.createElement('a');
            link.href = url;
            link.download = currentOffer.name;
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            
            document.getElementById('status').innerText = "Получение файла...";
        }
        currentOffer = null;
    }

    function uploadFile(toId) {
        if(!fileToSend) return;
        const xhr = new XMLHttpRequest();
        
        xhr.upload.onprogress = (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                updateProgress(percent);
                document.getElementById('status').innerText = "Отправка: " + percent + "% (" + formatBytes(e.loaded) + ")";
            }
        };

        xhr.open("POST", "/stream?to=" + toId + "&name=" + encodeURIComponent(fileToSend.name) + "&size=" + fileToSend.size);
        xhr.onload = () => {
            document.getElementById('status').innerText = "✅ Файл успешно отправлен!";
            setTimeout(() => { 
                document.getElementById('p-cont').style.display = 'none'; 
                document.getElementById('p-bar').style.width = '0%';
            }, 3000);
            fileToSend = null;
        };
        xhr.send(fileToSend);
    }
</script>
</body>
</html>
`)
}

// handleStream связывает POST-отправителя и GET-получателя через Pipe в реальном времени
func handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("to")
	mu.Lock()
	t, ok := transfers[id]
	if !ok {
		pr, pw := io.Pipe()
		t = &Transfer{pr: pr, pw: pw}
		transfers[id] = t
	}
	mu.Unlock()

	if r.Method == "POST" {
		// ОТПРАВИТЕЛЬ льет данные в PipeWriter
		io.Copy(t.pw, r.Body)
		t.pw.Close()
		mu.Lock()
		delete(transfers, id)
		mu.Unlock()
	} else {
		// ПОЛУЧАТЕЛЬ читает данные из PipeReader
		w.Header().Set("Content-Disposition", "attachment; filename="+r.URL.Query().Get("name"))
		w.Header().Set("Content-Length", r.URL.Query().Get("size"))
		io.Copy(w, t.pr)
	}
}
