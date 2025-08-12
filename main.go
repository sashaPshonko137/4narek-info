package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/gorilla/websocket"
)

const (
	token    = "7209712528:AAF7o20ysTcpgQb8JlVH4_CLmqH_iz5GiL8"
	chatID   = -4709535234 // Ваш чат ID
	timezone = "Asia/Tashkent"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	tgBot *bot.Bot
)

type ItemConfig struct {
	BasePrice    int
	NormalSales  int
	PriceStep    int
	AnalysisTime time.Duration
}

type DailyData struct {
	Date     string         `json:"date"`
	Prices   map[string]int `json:"prices"`
	BuyStats map[string]int `json:"buy_stats"`
	SellStats map[string]int `json:"sell_stats"`
	MessageID int           `json:"message_id"`
}

var (
	itemsConfig = map[string]ItemConfig{
		"sword5": {
			BasePrice:    2000000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 5 * time.Minute,
		},
		"sword6": {
			BasePrice:    2600000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 12 * time.Minute,
		},
		"sword7": {
			BasePrice:    3200000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
		},
		"pochti-megasword": {
			BasePrice:    3900000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 23 * time.Minute,
		},
		"megasword": {
			BasePrice:    5200000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 20 * time.Minute,
		},
		"elytra": {
			BasePrice:    1200000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 7 * time.Minute,
		},
		"elytra-mend": {
			BasePrice:    4500000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 15 * time.Minute,
		},
		"elytra-unbreak": {
			BasePrice:    1700000,
			NormalSales:  3,
			PriceStep:    100000,
			AnalysisTime: 9 * time.Minute,
		},

	}

	data struct {
		Prices    map[string]int
		BuyStats  map[string]int
		SellStats map[string]int
		LastTrade map[string]time.Time
	}
	dataMu sync.Mutex

	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex

	currentDay string
	dailyData  DailyData

	swordTimes = map[string]time.Time{
		"sword5": time.Now(),
		"sword6": time.Now(),
		"sword7": time.Now(),
		"pochti-megasword": time.Now(),
		"megasword": time.Now(),
	}
)

func main() {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("Error loading location: %v", err)
	}

	// Инициализация бота Telegram
	b, err := bot.New(token)
	if err != nil {
		log.Printf("Error creating bot: %v", err)
	}
	tgBot = b

	// Загрузка данных за сегодня
	loadDailyData(loc)

	// WebSocket сервер
	http.HandleFunc("/ws", handleConnections)
	go func() {
		log.Println("Server started on :8080")
		log.Print(http.ListenAndServe(":8080", nil))
	}()

	// Проверка смены дня
	go checkDayChange(loc)

	select {}
}

func loadDailyData(loc *time.Location) {
	dataMu.Lock()
	defer dataMu.Unlock()

	today := time.Now().In(loc).Format("2006-01-02")
	currentDay = today
	filename := fmt.Sprintf("data_%s.json", today)

	// Инициализация данных
	data.Prices = make(map[string]int)
	data.BuyStats = make(map[string]int)
	data.SellStats = make(map[string]int)
	data.LastTrade = make(map[string]time.Time)

	dailyData = DailyData{
		Date:     today,
		Prices:   make(map[string]int),
		BuyStats: make(map[string]int),
		SellStats: make(map[string]int),
	}

	// Загрузка из файла, если он существует и за сегодня
	if file, err := os.ReadFile(filename); err == nil {
		if err := json.Unmarshal(file, &dailyData); err == nil && dailyData.Date == today {
			// Копируем цены из сохраненных данных
			for item, price := range dailyData.Prices {
				data.Prices[item] = price
			}
			// Копируем статистику
			for item, count := range dailyData.BuyStats {
				data.BuyStats[item] = count
			}
			for item, count := range dailyData.SellStats {
				data.SellStats[item] = count
			}
			log.Println("Данные успешно загружены из файла")
		}
	}

	// Устанавливаем базовые цены для новых предметов
	for item, cfg := range itemsConfig {
		if _, exists := data.Prices[item]; !exists {
			data.Prices[item] = cfg.BasePrice
			dailyData.Prices[item] = cfg.BasePrice
		}
	}

	// Создаем/обновляем сообщение в Telegram
	updateTelegramMessage()
}

func checkDayChange(loc *time.Location) {
	for {
		now := time.Now().In(loc)
		nextDay := now.Add(24 * time.Hour)
		nextDay = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, loc)
		time.Sleep(time.Until(nextDay))

		// Новый день - сохраняем данные и создаем новое сообщение
		dataMu.Lock()
		saveDailyData()
		loadDailyData(loc) // Перезагружаем данные для нового дня
		dataMu.Unlock()
	}
}

func saveDailyData() {
	today := currentDay
	if today == "" {
		return
	}

	filename := fmt.Sprintf("data_%s.json", today)
	dailyData.Prices = data.Prices
	dailyData.BuyStats = data.BuyStats
	dailyData.SellStats = data.SellStats

	file, err := json.MarshalIndent(dailyData, "", "  ")
	if err != nil {
		log.Printf("Ошибка сохранения данных: %v", err)
		return
	}

	if err := os.WriteFile(filename, file, 0644); err != nil {
		log.Printf("Ошибка записи файла: %v", err)
		return
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
    ws, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print(err, " upgrade error")
		return
    }
    defer ws.Close()

    clientsMu.Lock()
    clients[ws] = true
    clientsMu.Unlock()

    // Отправляем текущие цены при подключении
    dataMu.Lock()
    ws.WriteJSON(data.Prices)
    dataMu.Unlock()

    for {
    	var msg struct {
    		Action string
    		Type   string
		}

		// Сначала читаем сырые данные
		var rawData json.RawMessage
		if err := ws.ReadJSON(&rawData); err != nil {
		    clientsMu.Lock()
		    delete(clients, ws)
 		  	 clientsMu.Unlock()
 		  	 log.Print(err, " readJson error")
 		   break
		}

		// Логируем сырой JSON
		log.Printf("Received JSON: %s", string(rawData))

		// Затем парсим в структуру
		if err := json.Unmarshal(rawData, &msg); err != nil {
 		   	log.Printf("Failed to unmarshal JSON: %v, raw data: %s", err, string(rawData))
  		  	break
		}

		dataMu.Lock()
        switch msg.Action {
        case "buy":
            data.BuyStats[msg.Type]++
            data.LastTrade[msg.Type] = time.Now()
            adjustPrice(msg.Type)
        case "sell":
            data.SellStats[msg.Type]++
            data.LastTrade[msg.Type] = time.Now()
            adjustPrice(msg.Type)
        case "info":
            ws.WriteJSON(data.Prices)
        }
        saveDailyData()
        dataMu.Unlock()
    }
}

func adjustPrice(item string) {
    cfg, ok := itemsConfig[item]
    if !ok {
        return
    }

    now := time.Now()
    buyCount := 0
    sellCount := 0

    // Считаем сделки за установленный период
    for t, action := range data.LastTrade {
        if now.Sub(action) > cfg.AnalysisTime {
            continue
        }
        if t == item {
            buyCount += data.BuyStats[t]
            sellCount += data.SellStats[t]
        }
    }

	if swordTimes[item].Add(itemsConfig[item].AnalysisTime).Before(time.Now()) {
    	if buyCount < itemsConfig[item].NormalSales {
			return
		}
		swordTimes[item] = time.Now()
	}

    // Изменяем цену по правилам
    newPrice := data.Prices[item]
    if buyCount > sellCount+cfg.NormalSales {
        newPrice -= cfg.PriceStep
    } else if buyCount < cfg.NormalSales {
        newPrice += cfg.PriceStep
    }

    // Применяем изменения
    if newPrice != data.Prices[item] {
        data.Prices[item] = newPrice
        dailyData.Prices[item] = newPrice

        // Рассылаем обновленные цены
        clientsMu.Lock()
        for client := range clients {
            client.WriteJSON(data.Prices)
        }
        clientsMu.Unlock()

        // Обновляем сообщение в Telegram
		
        updateTelegramMessage()
    }
}


func updateTelegramMessage() {
    // Получаем текущее время
    currentTime := time.Now().Format("2006-01-02 15:04:05")

    // Формируем текст сообщения с текущим временем
    msgText := fmt.Sprintf("📊 Статистика за %s\nПоследнее обновление: %s\n\n", dailyData.Date, currentTime)

    for item := range itemsConfig {
        msgText += fmt.Sprintf(
            "🔹 %s: %d ₽\n🛒 Куплено: %d\n💰 Продано: %d\n\n",
            item,
            data.Prices[item],
            data.BuyStats[item],
            data.SellStats[item],
        )
    }

    // Создаем новое сообщение или редактируем существующее
    ctx := context.Background()
    if dailyData.MessageID == 0 {
        msg, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
            ChatID: chatID,
            Text:   msgText,
        })
        if err == nil {
            dailyData.MessageID = msg.ID
            saveDailyData()
        }
    } else {
        _, err := tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
            ChatID:    chatID,
            MessageID: dailyData.MessageID,
            Text:      msgText,
        })
        if err != nil {
            log.Printf("Ошибка обновления сообщения: %v", err)
        }
    }
}