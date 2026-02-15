// Автоматически выбираем wss для https и ws для http
const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
const ws = new WebSocket(protocol + location.host + '/ws');

const notifySound = new Audio('https://actions.google.com');

let myId, currentOffer = null, fileToSend = null;

// Красивое форматирование размера файла
function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return parseFloat((bytes / Math.pow(1024, i)).toFixed(2)) + ' ' + ['Bytes', 'KB', 'MB', 'GB'][i];
}

function updateProgress(percent) {
    const cont = document.getElementById('p-cont');
    const bar = document.getElementById('p-bar');
    cont.style.display = 'block';
    bar.style.width = percent + '%';
}

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
            const userEl = document.createElement('div');
            userEl.className = 'user';
            userEl.innerHTML = `
                <span>${id} ${isMe ? '<strong>(Вы)</strong>' : ''}</span>
                ${!isMe ? `<button onclick="askSend('${id}')">Файл</button>` : ''}
            `;
            listDiv.appendChild(userEl);
        });
    }

    if(d.type === 'offer') {
        currentOffer = d;
        notifySound.play().catch(() => {});
        document.getElementById('notif-txt').innerHTML =
            `От: ${d.from}<br>Файл: <b>${d.name}</b><br>Размер: ${formatBytes(parseInt(d.size))}`;
        document.getElementById('notif').style.display = 'block';
    }

    if(d.type === 'accept') {
        uploadFile(d.from);
    }

    if (d.type === 'complete') {
        document.getElementById('status').innerText = "✅ Передача завершена!";
        setTimeout(() => {
            document.getElementById('status').innerText = "Ваш ID: " + myId;
            document.getElementById('p-cont').style.display = 'none';
        }, 4000);
    }

    // Добавим обработку сигнала о завершении от другого пользователя
    if (d.type === 'done') {
        document.getElementById('status').innerText = "✅ Передача завершена!";
        setTimeout(() => {
            document.getElementById('status').innerText = "Ваш ID: " + myId;
            document.getElementById('p-cont').style.display = 'none';
        }, 3000);
    }
};

function askSend(toId) {
    const input = document.getElementById('file-input');
    input.value = ''; // Сброс для повторной отправки
    input.onchange = () => {
        if (input.files.length === 0) return;
        fileToSend = input.files[0];
        ws.send(JSON.stringify({
            type: 'offer', to: toId, name: fileToSend.name, size: fileToSend.size.toString()
        }));
        document.getElementById('status').innerText = `Ждем подтверждения от ${toId}...`;
    };
    input.click();
}

function reply(ok) {
    const notif = document.getElementById('notif');
    notif.style.display = 'none';

    if(ok && currentOffer) {
        ws.send(JSON.stringify({type: 'accept', to: currentOffer.from}));

        // ПРОВЕРЬ: добавлен &from=${currentOffer.from}
        const url = `/stream?to=${myId}&from=${currentOffer.from}&name=${encodeURIComponent(currentOffer.name)}&size=${currentOffer.size}`;

        const a = document.createElement('a');
        a.href = url;
        a.download = currentOffer.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);

        document.getElementById('status').innerText = "📥 Получение файла...";
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
            document.getElementById('status').innerText = `Отправка: ${percent}%`;
        }
    };
    xhr.open("POST", `/stream?to=${toId}&name=${encodeURIComponent(fileToSend.name)}&size=${fileToSend.size}`);

    xhr.onload = () => {
        // Теперь здесь не пишем "Успешно", а пишем "Ожидание завершения..."
        document.getElementById('status').innerText = "📤 Файл в пути к получателю...";
        fileToSend = null;
    };
    xhr.send(fileToSend);
}
