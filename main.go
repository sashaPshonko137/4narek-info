package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type PriceAndRatio struct {
	Prices map[string]int     `json:"prices"`
	Ratios map[string]float64 `json:"ratios"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	tgBot *bot.Bot
)

var (
	clientItems   = make(map[*websocket.Conn]map[string]int)
	clientItemsMu sync.Mutex
)

var itemLimit = map[string]int{
	"netherite_sword": 140,
	"elytra": 24,
}

type ItemConfig struct {
	BasePrice    int
	NormalSales  int
	PriceStep    int
	AnalysisTime time.Duration
	MinPrice     int
	MaxPrice     int
	Type         string
}

var itemStockNorms = map[string]int{
    "sword5":  72,
    "sword6":  63,
    "sword7":  65,
    "pochti-megasword": 50,
    "megasword": 50,
    "elytra":  12,
    "elytra-mend": 4,
    "elytra-unbreak": 9,
}

type DailyData struct {
	Date       string             `json:"date"`
	Prices     map[string]int     `json:"prices"`
	Ratios     map[string]float64 `json:"ratios"` // 🆕
	BuyStats   map[string]int     `json:"buy_stats"`
	SellStats  map[string]int     `json:"sell_stats"`
	MessageID  int                `json:"message_id"`
}

var (
	
	itemsConfig = map[string]ItemConfig{
		"sword5": {
			BasePrice:    2100001,
			NormalSales:  10,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 500001,
			MaxPrice: 5000001,
			Type: "netherite_sword",
		},
		"sword6": {
			BasePrice:    2500002,
			NormalSales:  5,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 600002,
			MaxPrice: 6000002,
			Type: "netherite_sword",
		},
		"sword7": {
			BasePrice:    3500003,
			NormalSales:  8,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 700003,
			MaxPrice: 7000003,
			Type: "netherite_sword",
		},
		"pochti-megasword": {
			BasePrice:    3500004,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 1000004,
			MaxPrice: 8000004,
			Type: "netherite_sword",
		},
		"megasword": {
			BasePrice:    5000005,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 1200005,
			MaxPrice: 10000005,
			Type: "netherite_sword",
		},
		"elytra": {
			BasePrice:    1200006,
			NormalSales:  7,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 200006,
			MaxPrice: 30000006,
			Type: "elytra",
		},
		"elytra-mend": {
			BasePrice:    4500007,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 15 * time.Minute,
			MinPrice: 500007,
			MaxPrice: 8000007,
			Type: "elytra",
		},
		"elytra-unbreak": {
			BasePrice:    1700008,
			NormalSales:  4,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice: 300008,
			MaxPrice: 5000008,
			Type: "elytra",
		},

	}

data struct {
	Prices       map[string]int
	Ratios       map[string]float64 // 🆕
	BuyStats     map[string]int
	SellStats    map[string]int
	LastTrade    map[string]time.Time
	TradeHistory map[string][]TradeLog
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
		"elytra": time.Now(),
		"elytra-mend": time.Now(),
		"elytra-unbreak": time.Now(),
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
	//time.Sleep(1 * time.Minute)
	go fixPrice()

	select {}
}

func getConnectedClientsCount() int {
    clientsMu.Lock()
    defer clientsMu.Unlock()
    return len(clients)
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
data.TradeHistory = make(map[string][]TradeLog)
data.Ratios = make(map[string]float64)


	dailyData = DailyData{
		Date:     today,
		Prices:   make(map[string]int),
		BuyStats: make(map[string]int),
		SellStats: make(map[string]int),
		Ratios: make(map[string]float64),
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
	if _, exists := data.Ratios[item]; !exists {
		data.Ratios[item] = 0.8                // 🆕 стартовый коэффициент
		dailyData.Ratios[item] = 0.8           // 🆕
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
	dailyData.Prices = data.Prices
	dailyData.Ratios = data.Ratios

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

	clientItemsMu.Lock()
	clientItems[ws] = make(map[string]int)
	clientItemsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, ws)
		clientsMu.Unlock()

		clientItemsMu.Lock()
		delete(clientItems, ws)
		clientItemsMu.Unlock()
	}()

	// Отправляем текущие цены при подключении
	dataMu.Lock()
	ws.WriteJSON(PriceAndRatio{
	Prices: data.Prices,
	Ratios: data.Ratios,
})
	dataMu.Unlock()

	for {
		// Читаем сырые данные
		_, rawMsg, err := ws.ReadMessage()
		if err != nil {
			log.Printf("read error: %v", err)
			break
		}

		// Логируем входящий JSON
		log.Printf("[WS incoming] %s", string(rawMsg))

		// Парсим JSON в структуру
var msg struct {
	Action string            `json:"action"`
	Type   string            `json:"type"`   // для buy/sell
	Items  map[string]int    `json:"items"`  // для presence
}

		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("json unmarshal error: %v", err)
			continue
		}

		dataMu.Lock()
		switch msg.Action {
		case "buy":
			data.BuyStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "buy"})
			updateTelegramMessage()

	
		case "sell":
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "sell"})
			adjustPrice(msg.Type)
		case "info":
			ws.WriteJSON(PriceAndRatio{
		Prices: data.Prices,
		Ratios: data.Ratios,
	})
case "presence":
	clientItemsMu.Lock()
	clientItems[ws] = make(map[string]int)
	for item, count := range msg.Items {
		if count > 0 {
			clientItems[ws][item] = count
		}
	}
	clientItemsMu.Unlock()

		}
		saveDailyData()
		dataMu.Unlock()
	}
}




func getItemTypeCount(itemType string) int {
	clientItemsMu.Lock()
	defer clientItemsMu.Unlock()

	count := 0
	for _, items := range clientItems {
		for itemID, itemCount := range items {
			cfg, exists := itemsConfig[itemID]
			if !exists {
				continue
			}
			if cfg.Type == itemType {
				count += itemCount
			}
		}
	}
	return count
}


func fixPrice() {
	for {
		if getConnectedClientsCount() == 0 {
			log.Println("Нет подлключенных клиентов")
		} else {
			log.Println("fixing all prices ", time.Now().Format("15:04:05"))
			adjustPrice("sword5")
			adjustPrice("sword6")
			adjustPrice("sword7")
			adjustPrice("pochti-megasword")
			adjustPrice("elytra")
			adjustPrice("elytra-mend")
			adjustPrice("elytra-unbreak")
			adjustPrice("megasword")
		}
        time.Sleep(1 * time.Minute)
    }
}

type TradeLog struct {
	Time  time.Time
	Type  string // "buy" или "sell"
}

var swordTimesMu sync.Mutex

func countRecentSales(item string, since time.Time) int {
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "sell" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func getItemCount(item string) int {
	clientItemsMu.Lock()
	defer clientItemsMu.Unlock()

	count := 0
	for _, items := range clientItems {
		count += items[item]
	}
	return count
}

func countRecentBuys(item string, since time.Time) int {
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

var lastPriceUpdate = make(map[string]time.Time)

func adjustPrice(item string) {
	cfg, ok := itemsConfig[item]
	if !ok {
		return
	}

	now := time.Now()

	// Блокировка
	swordTimesMu.Lock()
	lastUpdate, updatedBefore := swordTimes[item]
	if updatedBefore && now.Sub(lastUpdate) < cfg.AnalysisTime {
		swordTimesMu.Unlock()
		return
	}
	swordTimes[item] = now
	swordTimesMu.Unlock()

	if !updatedBefore {
		lastUpdate = now.Add(-cfg.AnalysisTime)
	}

	// Получаем статистику
	sales := countRecentSales(item, lastUpdate)
	buys := countRecentBuys(item, lastUpdate)
	newPrice := data.Prices[item]
	priceBefore := newPrice
	ratioBefore := data.Ratios[item]
	ratio := ratioBefore

	// Определяем забитость
	totalItemStock := getItemCount(item)
	stockNorm := itemStockNorms[item]
	hasNorm := stockNorm > 0

	if sales >= cfg.NormalSales {
		if buys <= sales+2 {
			// Всё нормально, ничего не делаем
		} else {
			if ratio == 0.8 {
				ratio = 0.7
			} else {
				newPrice -= cfg.PriceStep
				if newPrice < cfg.MinPrice {
					newPrice = cfg.MinPrice
				}
			}
		}
	} else {
		isParasite := false

		// Поиск паразита
		if hasNorm && totalItemStock < stockNorm {
			for otherItem, otherCfg := range itemsConfig {
				if otherItem == item || otherCfg.Type != cfg.Type {
					continue
				}
				otherSales := countRecentSales(otherItem, lastUpdate)
				if otherSales < otherCfg.NormalSales && data.Prices[otherItem] > newPrice {
					isParasite = true
					break
				}
			}
		}

		if isParasite {
			// Не трогаем цену
		} else {
			if hasNorm && totalItemStock < stockNorm {
				if ratio == 0.7 {
					ratio = 0.8
				} else {
					newPrice += cfg.PriceStep
					if newPrice > cfg.MaxPrice {
						newPrice = cfg.MaxPrice
					}
				}
			} else {
				// Высокая забитость — понижаем цену
				newPrice -= cfg.PriceStep
				if newPrice < cfg.MinPrice {
					newPrice = cfg.MinPrice
				}
			}
		}
	}

	// Отправка Telegram отчета
	sendIntervalStatsToTelegram(
		item,
		lastUpdate,
		now,
		sales,
		cfg.NormalSales,
		priceBefore,
		newPrice,
	)

	// Обновление данных
	if newPrice != data.Prices[item] || ratio != ratioBefore {
		data.Prices[item] = newPrice
		dailyData.Prices[item] = newPrice
		data.Ratios[item] = ratio
		dailyData.Ratios[item] = ratio
		lastPriceUpdate[item] = now

		// Рассылаем клиентам
		clientsMu.Lock()
		for client := range clients {
			client.WriteJSON(data.Prices)
		}
		clientsMu.Unlock()

		updateTelegramMessage()
	}
}









func updateTelegramMessage() {
	currentTime := time.Now().Format("2006-01-02 15:04:05")

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

	ctx := context.Background()

	if dailyData.MessageID == 0 {
		msg, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   msgText,
		})
		if err != nil {
			log.Printf("[Telegram error] Не удалось отправить новое сообщение: %v", err)
			return
		}
		dailyData.MessageID = msg.ID
		saveDailyData()
	} else {
		_, err := tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: dailyData.MessageID,
			Text:      msgText,
		})
		if err != nil {
			log.Printf("[Telegram error] Не удалось обновить сообщение: %v", err)

			// Попробуем отправить заново, если редактирование не удалось (например, сообщение удалено)
			msg, sendErr := tgBot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   msgText,
			})
			if sendErr == nil {
				dailyData.MessageID = msg.ID
				saveDailyData()
			} else {
				log.Printf("[Telegram error] Повторная отправка тоже не удалась: %v", sendErr)
			}
		}
	}
}

func sendIntervalStatsToTelegram(item string, start, end time.Time, actualSales, expectedSales, priceBefore, priceAfter int) {
	status := "✅"
	if actualSales < expectedSales {
		status = "⚠️"
	}

	// 1. Получаем онлайн с внешнего сервера
	onlineCount := getOnlineCount()

	// 2. Подсчитываем покупки за интервал
	buyCount := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(start) && trade.Time.Before(end) {
			buyCount++
		}
	}

	// 3. Получаем количество предметов на руках у клиентов
	onHand := getItemCount(item)

	// 4. Формируем сообщение
	msg := fmt.Sprintf(
		"*%s* %s\n"+
			"⏳ Интервал: %s - %s\n"+
			"📦 Покупки: *%d*\n"+
			"📊 Продажи: *%d* из *%d* (норма)\n"+
			"💸 Цена: %d → %d\n"+
			"🧮 Коэффициент: %.2f\n"+
			"🎒 На ah: %d\n"+
			"👥 Онлайн: %d игроков",
		item,
		status,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		buyCount,
		actualSales,
		expectedSales,
		priceBefore,
		priceAfter,
		data.Ratios[item],
		onHand,        // добавлено
		onlineCount,
	)

	// 5. Отправляем в Telegram
	ctx := context.Background()
	_, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    -4633184325,
		Text:      msg,
		ParseMode: "Markdown",
	})
	if err != nil {
		log.Printf("[Telegram] Ошибка при отправке интервал-статы: %v", err)
	}

	// 6. Сохраняем лог в файл (без Markdown)
	plainLog := fmt.Sprintf(
		"%s [%s → %s] %s | Покупки: %d | Продажи: %d/%d | Цена: %d→%d | На руках: %d | Онлайн: %d\n",
		item,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		status,
		buyCount,
		actualSales,
		expectedSales,
		priceBefore,
		priceAfter,
		onHand,        // добавлено
		onlineCount,
	)

	appendToFile("logs_interval.txt", plainLog)
}




func appendToFile(filename, content string) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Ошибка открытия файла лога: %v", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		log.Printf("Ошибка записи в файл лога: %v", err)
	}
}

func getOnlineCount() int {
	resp, err := http.Get("http://45.141.76.110:5000/status")
	if err != nil {
		log.Printf("Ошибка запроса онлайна: %v", err)
		return -1
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Ошибка чтения тела ответа: %v", err)
		return -1
	}

	var status struct {
		PlayersOnline int `json:"players_online"`
	}

	if err := json.Unmarshal(body, &status); err != nil {
		log.Printf("Ошибка парсинга JSON онлайна: %v", err)
		return -1
	}

	return status.PlayersOnline
}