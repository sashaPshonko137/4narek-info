package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/go-telegram/bot"
)

const (
	token    = "7209712528:AAF7o20ysTcpgQb8JlVH4_CLmqH_iz5GiL8"
	chatID   = -4709535234
	timezone = "Asia/Tashkent"
)

type Swords struct {
	Sword5nmSell            int `json:"sword5nm_sell"`
	Sword7nmSell            int `json:"sword7nm_sell"`
	Sword5Sell              int `json:"sword5_sell"`
	Sword6Sell              int `json:"sword6_sell"`
	Sword7Sell              int `json:"sword7_sell"`
	MegaswordSell           int `json:"megasword_sell"`
	NetheriteLeggingsSell   int `json:"netherite_leggings_sell"`
	NetheriteChestplateSell int `json:"netherite_chestplate_sell"`
	NetheriteHelmetSell     int `json:"netherite_helmet_sell"`
	NetheriteBootsSell      int `json:"netherite_boots_sell"`
	Sword5nmBuy             int `json:"sword5nm_buy"`
	Sword7nmBuy             int `json:"sword7nm_buy"`
	Sword5Buy               int `json:"sword5_buy"`
	Sword6Buy               int `json:"sword6_buy"`
	Sword7Buy               int `json:"sword7_buy"`
	MegaswordBuy            int `json:"megasword_buy"`
	NetheriteLeggingsBuy    int `json:"netherite_leggings_buy"`
	NetheriteChestplateBuy  int `json:"netherite_chestplate_buy"`
	NetheriteHelmetBuy      int `json:"netherite_helmet_buy"`
	NetheriteBootsBuy       int `json:"netherite_boots_buy"`
}

type DailyData struct {
	Date      string `json:"date"`
	MessageID int    `json:"message_id"`
	Swords    Swords `json:"swords"`
	LastText  string `json:"last_text"` // Store last message text to avoid "not modified" errors
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

	// Set up HTTP handlers
	http.HandleFunc("/sell", sellHandler)
	http.HandleFunc("/buy", buyHandler)

	// Start single HTTP server
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
			Date:   today,
			Swords: Swords{},
		}
		return
	}

	if err := json.Unmarshal(file, &data); err != nil {
		log.Printf("Error decoding data file: %v", err)
		data = DailyData{
			Date:   today,
			Swords: Swords{},
		}
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

func sellHandler(w http.ResponseWriter, r *http.Request) {
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

	switch request.Type {
	case "5nomend":
		data.Swords.Sword5nmSell++
	case "7nomend":
		data.Swords.Sword7nmSell++
	case "sword5":
		data.Swords.Sword5Sell++
	case "sword6":
		data.Swords.Sword6Sell++
	case "sword7":
		data.Swords.Sword7Sell++
	case "megasword":
		data.Swords.MegaswordSell++
	case "бошмаки":
		data.Swords.NetheriteBootsSell++
	case "шлем":
		data.Swords.NetheriteHelmetSell++
	case "нагрудник":
		data.Swords.NetheriteChestplateSell++
	case "штаны":
		data.Swords.NetheriteLeggingsSell++
	default:
		http.Error(w, "Invalid sword type", http.StatusBadRequest)
		return
	}

	saveData()
	updateTelegramMessage(context.Background())

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data.Swords)
}

func buyHandler(w http.ResponseWriter, r *http.Request) {
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

	switch request.Type {
	case "5nomend":
		data.Swords.Sword5nmBuy++
	case "7nomend":
		data.Swords.Sword7nmBuy++
	case "sword5":
		data.Swords.Sword5Buy++
	case "sword6":
		data.Swords.Sword6Buy++
	case "sword7":
		data.Swords.Sword7Buy++
	case "megasword":
		data.Swords.MegaswordBuy++
	case "бошмаки":
		data.Swords.NetheriteBootsBuy++
	case "шлем":
		data.Swords.NetheriteHelmetBuy++
	case "нагрудник":
		data.Swords.NetheriteChestplateBuy++
	case "штаны":
		data.Swords.NetheriteLeggingsBuy++
	default:
		http.Error(w, "Invalid sword type", http.StatusBadRequest)
		return
	}

	saveData()
	updateTelegramMessage(context.Background())

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data.Swords)
}

func updateTelegramMessage(ctx context.Context) {
	newText := generateMessageText()
	if newText == data.LastText {
		return // Skip update if text hasn't changed
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
  return fmt.Sprintf("🗡 Статистика за %s:\n\n"+
    "5nm: %d/%d\n5: %d/%d \n"+
    "6: %d/%d\n+7nm: %d/%d\n"+
    "7: %d/%d\nMEGA: %d/%d\n\n"+
    "Ботинки: %d/%d\nШлем: %d/%d\n"+
    "Нагрудник: %d/%d\nШтаны: %d/%d",
    today,
    data.Swords.Sword5nmBuy, data.Swords.Sword5nmSell, 
    data.Swords.Sword5Buy, data.Swords.Sword5Sell, 
    data.Swords.Sword6Buy, data.Swords.Sword6Sell,
    data.Swords.Sword7nmBuy, data.Swords.Sword7nmSell,
    data.Swords.Sword7Buy, data.Swords.Sword7Sell, 
    data.Swords.MegaswordBuy, data.Swords.MegaswordSell,
    data.Swords.NetheriteBootsBuy, data.Swords.NetheriteBootsSell, 
    data.Swords.NetheriteHelmetBuy, data.Swords.NetheriteHelmetSell,
    data.Swords.NetheriteChestplateBuy, data.Swords.NetheriteChestplateSell, 
    data.Swords.NetheriteLeggingsBuy, data.Swords.NetheriteLeggingsSell)
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
				Date:   time.Now().In(loc).Format("2006-01-02"),
				Swords: Swords{},
			}
			dataMutex.Unlock()

			initTelegramMessage(ctx)

		case <-ctx.Done():
			return
		}
	}
}