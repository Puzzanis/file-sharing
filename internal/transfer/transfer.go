package transfer

import (
	"io"
	"sync"
)

// Transfer структура для передачи файла между пользователями
type Transfer struct {
	Pr *io.PipeReader
	Pw *io.PipeWriter
}

var (
	Files = make(map[string]*Transfer)
	Mu    sync.Mutex
)

func GetOrCreate(id string) *Transfer {
	Mu.Lock()
	defer Mu.Unlock()
	if t, ok := Files[id]; ok {
		return t
	}
	pr, pw := io.Pipe()
	t := &Transfer{Pr: pr, Pw: pw}
	Files[id] = t
	return t
}

func Delete(id string) {
	Mu.Lock()
	delete(Files, id)
	Mu.Unlock()
}
