package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Transfer struct {
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	fileName   chan string
	fileSize   chan int64
}

var (
	transfers = make(map[string]*Transfer)
	mu        sync.Mutex
)

// Генерация случайного кода из 4 цифр
func generateCode() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%04d", rand.Intn(10000))
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/get-code", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, generateCode())
	})

	fmt.Println("Сервер обмена запущен на http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `
		<style>
			body { font-family: sans-serif; display: flex; justify-content: center; padding-top: 50px; background: #f4f7f6; }
			.card { background: white; padding: 2rem; border-radius: 12px; box-shadow: 0 4px 15px rgba(0,0,0,0.1); width: 350px; }
			input, button { width: 100%; padding: 12px; margin: 8px 0; border-radius: 6px; border: 1px solid #ddd; box-sizing: border-box; }
			button { background: #007bff; color: white; border: none; cursor: pointer; font-weight: bold; }
			button:hover { background: #0056b3; }
			.code-display { font-size: 24px; font-weight: bold; text-align: center; color: #007bff; margin: 10px 0; letter-spacing: 5px; }
			#progress-container { width: 100%; background: #eee; display: none; height: 8px; border-radius: 4px; overflow: hidden; }
			#progress-bar { width: 0%; height: 100%; background: #28a745; transition: width 0.2s; }
		</style>

		<div class="card">
			<h2 style="text-align:center">File Drop</h2>
			
			<div id="setup-area">
				<button onclick="initSend()" style="background:#28a745">Я хочу отправить файл</button>
				<button onclick="document.getElementById('receive-area').style.display='block'; this.style.display='none'">У меня есть код</button>
			</div>

			<div id="send-area" style="display:none">
				<p>Ваш код для получателя:</p>
				<div id="my-code" class="code-display">----</div>
				<input type="file" id="file">
				<button onclick="startUpload()">Начать передачу</button>
			</div>

			<div id="receive-area" style="display:none">
				<input type="text" id="join-code" placeholder="Введите 4 цифры">
				<button onclick="startDownload()">Скачать файл</button>
			</div>

			<div id="progress-container"><div id="progress-bar"></div></div>
			<p id="status" style="text-align:center; font-size: 14px; color: #666"></p>
		</div>

		<script>
			let activeCode = "";

			async function initSend() {
				const res = await fetch('/get-code');
				activeCode = await res.text();
				document.getElementById('my-code').innerText = activeCode;
				document.getElementById('send-area').style.display = 'block';
				document.getElementById('setup-area').style.display = 'none';
			}

			function startUpload() {
				const fileInput = document.getElementById('file');
				if(!fileInput.files[0]) return alert("Выберите файл!");
				
				const file = fileInput.files[0];
				const xhr = new XMLHttpRequest();
				const bar = document.getElementById('progress-bar');
				document.getElementById('progress-container').style.display = 'block';

				xhr.upload.onprogress = (e) => {
					const percent = Math.round((e.loaded / e.total) * 100);
					bar.style.width = percent + '%';
					document.getElementById('status').innerText = "Отправка: " + percent + "%";
				};

				xhr.open("POST", "/stream?code=" + activeCode + "&name=" + encodeURIComponent(file.name) + "&size=" + file.size);
				xhr.send(file);
				document.getElementById('status').innerText = "Ожидание подключения получателя...";
			}

			function startDownload() {
				const code = document.getElementById('join-code').value;
				if(!code) return alert("Введите код!");
				window.location.href = "/stream?code=" + code;
			}
		</script>
	`)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	// Код функции handleStream остается таким же, как в предыдущем ответе
	code := r.URL.Query().Get("code")
	mu.Lock()
	t, exists := transfers[code]
	if !exists {
		pr, pw := io.Pipe()
		t = &Transfer{
			pipeReader: pr,
			pipeWriter: pw,
			fileName:   make(chan string, 1),
			fileSize:   make(chan int64, 1),
		}
		transfers[code] = t
	}
	mu.Unlock()

	if r.Method == http.MethodPost {
		t.fileName <- r.URL.Query().Get("name")
		var size int64
		fmt.Sscanf(r.URL.Query().Get("size"), "%d", &size)
		t.fileSize <- size

		io.Copy(t.pipeWriter, r.Body)
		t.pipeWriter.Close()

		mu.Lock()
		delete(transfers, code)
		mu.Unlock()
	} else {
		select {
		case name := <-t.fileName:
			size := <-t.fileSize
			w.Header().Set("Content-Disposition", "attachment; filename="+name)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
			io.Copy(w, t.pipeReader)
		case <-time.After(30 * time.Minute): // Тайм-аут, если никто не пришел
			http.Error(w, "Transfer expired", 404)
		}
	}
}
