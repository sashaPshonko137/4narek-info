package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"time"

	"github.com/go-telegram/bot"
)

const (
	token    = "7209712528:AAF7o20ysTcpgQb8JlVH4_CLmqH_iz5GiL8"
	chatID   = -4709535234
	timezone = "Asia/Tashkent"
)

type DailyData struct {
	Date      string          `json:"date"`
	MessageID int             `json:"message_id"`
	BuyMap    map[string]int  `json:"buy_map"`
	SellMap   map[string]int  `json:"sell_map"`
	LastText  string          `json:"last_text"`
}

var (
	tgBot     *bot.Bot
	data      DailyData
	dataMutex sync.Mutex
	loc       *time.Location
)

func init() {
	var err error
	loc, err = time.LoadLocation(timezone)
	if err != nil {
		log.Fatalf("Error loading location: %v", err)
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	loadData()

	b, err := bot.New(token)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}
	tgBot = b

	initTelegramMessage(ctx)
	http.HandleFunc("/buy_shue", buyShueHandler)
	http.HandleFunc("/sell_shue", SellShueHandler)

	go func() {
		log.Println("Server started on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	go dailyResetChecker(ctx)

	<-ctx.Done()
}

func loadData() {
	today := time.Now().In(loc).Format("2006-01-02")
	filename := fmt.Sprintf("data_%s.json", today)

	file, err := os.ReadFile(filename)
	if err != nil {
		data = DailyData{
			Date:    today,
			BuyMap:  make(map[string]int), // Явная инициализация карты
			SellMap: make(map[string]int), // Явная инициализация карты
		}
		return
	}

	if err := json.Unmarshal(file, &data); err != nil {
		log.Printf("Error decoding data file: %v", err)
		data = DailyData{
			Date:    today,
			BuyMap:  make(map[string]int), // Явная инициализация карты
			SellMap: make(map[string]int), // Явная инициализация карты
		}
	}

	// Дополнительная проверка на nil мапы после загрузки
	if data.BuyMap == nil {
		data.BuyMap = make(map[string]int)
	}
	if data.SellMap == nil {
		data.SellMap = make(map[string]int)
	}
}

func saveData() {
	today := time.Now().In(loc).Format("2006-01-02")
	filename := fmt.Sprintf("data_%s.json", today)

	file, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Error encoding data: %v", err)
		return
	}

	if err := os.WriteFile(filename, file, 0644); err != nil {
		log.Printf("Error saving data: %v", err)
	}
}

func initTelegramMessage(ctx context.Context) {
	if data.MessageID == 0 {
		msgText := generateMessageText()
		msg, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   msgText,
		})
		if err != nil {
			log.Printf("Error sending message: %v", err)
			return
		}
		data.MessageID = msg.ID
		data.LastText = msgText
		saveData()
	} else {
		updateTelegramMessage(ctx)
	}
}

func buyShueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dataMutex.Lock()
	defer dataMutex.Unlock()

	// Проверка инициализации карты перед использованием
	if data.BuyMap == nil {
		data.BuyMap = make(map[string]int)
	}

	data.BuyMap[request.Type]++

	saveData()
	updateTelegramMessage(context.Background())

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data.BuyMap)
}

func SellShueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dataMutex.Lock()
	defer dataMutex.Unlock()

	// Проверка инициализации карты перед использованием
	if data.SellMap == nil {
		data.SellMap = make(map[string]int)
	}

	data.SellMap[request.Type]++

	saveData()
	updateTelegramMessage(context.Background())

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data.SellMap)
}

func updateTelegramMessage(ctx context.Context) {
	newText := generateMessageText()
	if newText == data.LastText {
		return
	}

	_, err := tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: data.MessageID,
		Text:      newText,
	})
	if err != nil {
		log.Printf("Error updating Telegram message: %v", err)
		return
	}

	data.LastText = newText
	saveData()
}

func generateMessageText() string {
	today := time.Now().In(loc).Format("2006-01-02")
	currentTime := time.Now().In(loc).Format("15:04")
	
	msg := fmt.Sprintf("🗡 Статистика за %s:\n"+
		today,)

	// Обработка данных из мап
	if len(data.BuyMap) > 0 || len(data.SellMap) > 0 {
		msg += "\nДополнительные предметы:\n"
		
		// Собираем все уникальные ключи из обеих мап
		allKeys := make([]string, 0)
		for k := range data.BuyMap {
			allKeys = append(allKeys, k)
		}
		for k := range data.SellMap {
			if _, exists := data.BuyMap[k]; !exists {
				allKeys = append(allKeys, k)
			}
		}
		
		// Сортируем ключи по алфавиту
		sort.Strings(allKeys)
		
		// Выводим каждый предмет в формате "название: покупки/продажи"
		for _, item := range allKeys {
			buyCount := data.BuyMap[item]
			sellCount := data.SellMap[item]
			msg += fmt.Sprintf("%s: %d/%d\n", item, buyCount, sellCount)
		}
	}

	// Добавляем время в конце
	msg += fmt.Sprintf("\n%s", currentTime)

	return msg
}

func dailyResetChecker(ctx context.Context) {
	for {
		now := time.Now().In(loc)
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
		duration := nextMidnight.Sub(now)

		select {
		case <-time.After(duration):
			dataMutex.Lock()
			data = DailyData{
				Date:    time.Now().In(loc).Format("2006-01-02"),
				BuyMap:  make(map[string]int),
				SellMap: make(map[string]int),
			}
			dataMutex.Unlock()

			initTelegramMessage(ctx)

		case <-ctx.Done():
			return
		}
	}
}