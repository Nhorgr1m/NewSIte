package main

import (
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type School struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Participant struct {
	ID         int
	FullName   string
	SchoolID   int
	Phone      string
	Profession string
	CreatedAt  time.Time
}

var (
	schools      []School
	participants []Participant
	mu           sync.Mutex
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func loadSchools() {
	data, err := os.ReadFile("schools.json")
	if err != nil {
		log.Fatal("не найден schools.json: ", err)
	}
	if err := json.Unmarshal(data, &schools); err != nil {
		log.Fatal("ошибка в schools.json: ", err)
	}
}

func schoolName(id int) (string, bool) {
	for _, s := range schools {
		if s.ID == id {
			return s.Name, true
		}
	}
	return "", false
}

func listSchools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, schools)
}

func normalizePhone(s string) (string, bool) {
	var d []rune
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d = append(d, r)
		}
	}
	if len(d) == 11 && (d[0] == '7' || d[0] == '8') {
		d = d[1:]
	}
	if len(d) != 10 {
		return "", false
	}
	return "+7" + string(d), true
}

type startInput struct {
	FullName string `json:"full_name"`
	SchoolID int    `json:"school_id"`
	Phone    string `json:"phone"` 
}

func start(w http.ResponseWriter, r *http.Request) {
	var in startInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Неверные данные"})
		return
	}
	in.FullName = strings.TrimSpace(in.FullName)
	if in.FullName == "" {
		writeJSON(w, 400, map[string]string{"error": "Введите ФИО"})
		return
	}
	if _, ok := schoolName(in.SchoolID); !ok {
		writeJSON(w, 400, map[string]string{"error": "Выберите школу"})
		return
	}

	phone := strings.TrimSpace(in.Phone)
	if phone != "" {
		norm, ok := normalizePhone(phone)
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "Проверьте номер телефона"})
			return
		}
		phone = norm
	}

	mu.Lock()
	p := Participant{
		ID:        len(participants) + 1,
		FullName:  in.FullName,
		SchoolID:  in.SchoolID,
		Phone:     phone,
		CreatedAt: time.Now(),
	}
	participants = append(participants, p)
	mu.Unlock()

	writeJSON(w, 201, map[string]int{"participant_id": p.ID})
}

// пароль для страницы выгрузки поменяйте на свой
const adminPassword = "zhd-2026-ChangeMe"

type resultInput struct {
	ParticipantID int    `json:"participant_id"`
	Profession    string `json:"profession"`
}

// тест завершён: сохраняем профессию участника
func saveResult(w http.ResponseWriter, r *http.Request) {
	var in resultInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad input"})
		return
	}
	prof := []rune(strings.TrimSpace(in.Profession))
	if len(prof) > 200 {
		prof = prof[:200]
	}

	mu.Lock()
	defer mu.Unlock()
	if in.ParticipantID < 1 || in.ParticipantID > len(participants) {
		writeJSON(w, 404, map[string]string{"error": "участник не найден"})
		return
	}
	participants[in.ParticipantID-1].Profession = string(prof)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// выгрузка: пароль приходит со страницы /admin.html
func export(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("password") != adminPassword {
		time.Sleep(time.Second)
		http.Error(w, "Неверный пароль", 403)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="participants.csv"`)
	w.Write([]byte("\xEF\xBB\xBF"))

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	defer cw.Flush()
	cw.Write([]string{"ФИО", "Школа", "Телефон", "Профессия", "Дата и время"})

	mu.Lock()
	defer mu.Unlock()
	for _, p := range participants {
		name, _ := schoolName(p.SchoolID)
		cw.Write([]string{
			p.FullName, name, p.Phone, p.Profession,
			p.CreatedAt.Format("02.01.2006 15:04"),
		})
	}
}

func main() {
	loadSchools()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /schools", listSchools)
	mux.HandleFunc("POST /start", start)
	mux.HandleFunc("POST /result", saveResult)
	mux.HandleFunc("POST /export", export)
	mux.Handle("/", http.FileServer(http.Dir("static")))

	log.Println("Сервер: http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
