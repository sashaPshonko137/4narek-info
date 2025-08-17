package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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
	clientItems     = make(map[*websocket.Conn]map[string]int)
	clientInventory = make(map[*websocket.Conn]map[string]int)
)

var itemLimit = map[string]int{
	"netherite_sword": 96,
	"elytra":          24,
}

var inventoryLimit = map[string]int{
	"netherite_sword": 28 * 3 * 4,
	"elytra":          28 * 3,
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

type DailyData struct {
	Date         string             `json:"date"`
	Prices       map[string]int     `json:"prices"`
	Ratios       map[string]float64 `json:"ratios"`
	BuyStats     map[string]int     `json:"buy_stats"`
	SellStats    map[string]int     `json:"sell_stats"`
	TrySellStats map[string]int     `json:"try_sell_stats"`
	MessageID    int                `json:"message_id"`
}

var (
	itemsConfig = map[string]ItemConfig{
		"sword5": {
			BasePrice:    2500001,
			NormalSales:  5,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500001,
			MaxPrice:     5000001,
			Type:         "netherite_sword",
		},
		"sword6": {
			BasePrice:    2900002,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600002,
			MaxPrice:     6000002,
			Type:         "netherite_sword",
		},
		"sword7": {
			BasePrice:    4600003,
			NormalSales:  4,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     700003,
			MaxPrice:     7000003,
			Type:         "netherite_sword",
		},
		"sword5-unbreak": {
			BasePrice:    2500004,
			NormalSales:  5,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500004,
			MaxPrice:     5000004,
			Type:         "netherite_sword",
		},
		"sword6-unbreak": {
			BasePrice:    2900005,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600005,
			MaxPrice:     6000005,
			Type:         "netherite_sword",
		},
		"sword7-unbreak": {
			BasePrice:    4600006,
			NormalSales:  4,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     700006,
			MaxPrice:     7000006,
			Type:         "netherite_sword",
		},
		"pochti-megasword": {
			BasePrice:    7000007,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1000007,
			MaxPrice:     8000007,
			Type:         "netherite_sword",
		},
		"megasword": {
			BasePrice:    7400008,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1200008,
			MaxPrice:     10000008,
			Type:         "netherite_sword",
		},
		"elytra": {
			BasePrice:    1200009,
			NormalSales:  11,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     200009,
			MaxPrice:     30000009,
			Type:         "elytra",
		},
		"elytra-unbreak": {
			BasePrice:    2700010,
			NormalSales:  5,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     300010,
			MaxPrice:     5000010,
			Type:         "elytra",
		},
	}
)

type TradeLog struct {
	Time time.Time
	Type string // "buy", "sell" или "try-sell"
}

type Data struct {
	Prices       map[string]int
	Ratios       map[string]float64
	BuyStats     map[string]int
	SellStats    map[string]int
	TrySellStats map[string]int
	LastTrade    map[string]time.Time
	TradeHistory map[string][]TradeLog
}

var (
	data   = &Data{}
	mutex  = sync.Mutex{} // Единственный мьютекс для всей системы

	clients = make(map[*websocket.Conn]bool)

	currentDay string
	dailyData  DailyData

	swordTimes = make(map[string]time.Time)

	lastPriceUpdate = make(map[string]time.Time)
)

func main() {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("Error loading location: %v", err)
		os.Exit(1)
	}

	// Инициализация бота Telegram
	b, err := bot.New(token)
	if err != nil {
		log.Printf("Error creating bot: %v", err)
		os.Exit(1)
	}
	tgBot = b

	// Инициализация данных
	data.Prices = make(map[string]int)
	data.BuyStats = make(map[string]int)
	data.SellStats = make(map[string]int)
	data.TrySellStats = make(map[string]int)
	data.LastTrade = make(map[string]time.Time)
	data.TradeHistory = make(map[string][]TradeLog)
	data.Ratios = make(map[string]float64)

	// Загрузка данных за сегодня
	loadDailyData(loc)
	updateTelegramMessageSimple()

	// WebSocket сервер
	http.HandleFunc("/ws", handleConnections)
	go func() {
		log.Println("Server started on :8080")
		log.Print(http.ListenAndServe(":8080", nil))
	}()

	// Проверка смены дня
	go checkDayChange(loc)
	go fixPrice()
	go startStatsSender()

	select {}
}

func getConnectedClientsCount() int {
	mutex.Lock()
	defer mutex.Unlock()
	return len(clients)
}

func loadDailyData(loc *time.Location) {
	mutex.Lock()
	defer mutex.Unlock()

	today := time.Now().In(loc).Format("2006-01-02")
	currentDay = today
	filename := fmt.Sprintf("data_%s.json", today)

	// Инициализация данных
	dailyData = DailyData{
		Date:         today,
		Prices:       make(map[string]int),
		BuyStats:     make(map[string]int),
		SellStats:    make(map[string]int),
		TrySellStats: make(map[string]int),
		Ratios:       make(map[string]float64),
	}

	// Загрузка из файла, если он существует и за сегодня
	if file, err := os.ReadFile(filename); err == nil {
		if err := json.Unmarshal(file, &dailyData); err == nil && dailyData.Date == today {
			// Копируем данные из сохраненных данных
			for item, price := range dailyData.Prices {
				data.Prices[item] = price
			}
			for item, count := range dailyData.BuyStats {
				data.BuyStats[item] = count
			}
			for item, count := range dailyData.SellStats {
				data.SellStats[item] = count
			}
			for item, count := range dailyData.TrySellStats {
				data.TrySellStats[item] = count
			}
			for item, ratio := range dailyData.Ratios {
				data.Ratios[item] = ratio
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
			data.Ratios[item] = 0.8
			dailyData.Ratios[item] = 0.8
		}
	}

	// Инициализация времени последнего обновления
	for item := range itemsConfig {
		swordTimes[item] = time.Now()
	}

	// Сохраняем данные
	saveDailyDataNoMessageUpdate()
}

func saveDailyDataNoMessageUpdate() {
	// Эта функция вызывается с уже заблокированным mutex
	today := currentDay
	if today == "" {
		return
	}

	filename := fmt.Sprintf("data_%s.json", today)
	dailyData.Prices = data.Prices
	dailyData.BuyStats = data.BuyStats
	dailyData.SellStats = data.SellStats
	dailyData.TrySellStats = data.TrySellStats
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
func updateTelegramMessageWithoutLocks(prices, buyStats, sellStats map[string]int, date string, messageID int) {
    currentTime := time.Now().Format("2006-01-02 15:04:05")

    msgText := fmt.Sprintf("📊 Статистика за %s\nПоследнее обновление: %s\n\n", date, currentTime)

    for item := range itemsConfig {
        msgText += fmt.Sprintf(
            "🔹 %s: %d ₽\n🛒 Куплено: %d\n💰 Продано: %d\n\n",
            item,
            prices[item],
            buyStats[item],
            sellStats[item],
        )
    }

    ctx := context.Background()
    var err error

    if messageID == 0 {
        // Отправляем новое сообщение
        msg, sendErr := tgBot.SendMessage(ctx, &bot.SendMessageParams{
            ChatID: chatID,
            Text:   msgText,
        })
        if sendErr != nil {
            log.Printf("[Telegram error] Не удалось отправить сообщение: %v", sendErr)
            return
        }
        messageID = msg.ID
    } else {
        // Пытаемся обновить существующее сообщение
        _, err = tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
            ChatID:    chatID,
            MessageID: messageID,
            Text:      msgText,
        })
        if err != nil {
            log.Printf("[Telegram error] Не удалось обновить сообщение: %v", err)
            return
        }
    }

    // Обновляем messageID если он изменился
    if messageID != dailyData.MessageID {
        mutex.Lock()
        dailyData.MessageID = messageID
        saveDailyDataNoMessageUpdate()
        mutex.Unlock()
    }
}
func updateTelegramMessageSimple() {
	mutex.Lock()
	// Создаем копии данных для использования вне блокировки
	prices := make(map[string]int)
	buyStats := make(map[string]int)
	sellStats := make(map[string]int)
	date := dailyData.Date
	messageID := dailyData.MessageID

	for k, v := range data.Prices {
		prices[k] = v
	}
	for k, v := range data.BuyStats {
		buyStats[k] = v
	}
	for k, v := range data.SellStats {
		sellStats[k] = v
	}
	mutex.Unlock()

	// Обновляем Telegram сообщение
	updateTelegramMessageWithoutLocks(prices, buyStats, sellStats, date, messageID)
}

func checkDayChange(loc *time.Location) {
	for {
		now := time.Now().In(loc)
		nextDay := now.Add(24 * time.Hour)
		nextDay = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, loc)
		time.Sleep(time.Until(nextDay))

		// Новый день - сохраняем данные и создаем новое сообщение
		mutex.Lock()
		saveDailyData()
		mutex.Unlock()

		loadDailyData(loc) // Перезагружаем данные для нового дня
	}
}

func saveDailyData() {
	// Эта функция вызывается с уже заблокированным mutex
	today := currentDay
	if today == "" {
		return
	}

	filename := fmt.Sprintf("data_%s.json", today)
	dailyData.Prices = data.Prices
	dailyData.BuyStats = data.BuyStats
	dailyData.SellStats = data.SellStats
	dailyData.TrySellStats = data.TrySellStats
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

	mutex.Lock()
	clients[ws] = true
	mutex.Unlock()

	mutex.Lock()
	clientItems[ws] = make(map[string]int)
	clientInventory[ws] = make(map[string]int)
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(clients, ws)
		delete(clientItems, ws)
		delete(clientInventory, ws)
		mutex.Unlock()
	}()

	// Отправляем текущие цены при подключении
	mutex.Lock()
	err = ws.WriteJSON(PriceAndRatio{
		Prices: data.Prices,
		Ratios: data.Ratios,
	})
	mutex.Unlock()
	if err != nil {
		log.Printf("write error: %v", err)
		return
	}

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
			Action    string         `json:"action"`
			Type      string         `json:"type"`   // для buy/sell
			Items     map[string]int `json:"items"`  // для presence
			Inventory map[string]int `json:"inventory"`
		}

		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("json unmarshal error: %v", err)
			continue
		}

		mutex.Lock()
		switch msg.Action {
		case "buy":
			data.BuyStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "buy"})
			mutex.Unlock()
			updateTelegramMessage()

		case "sell":
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "sell"})
			mutex.Unlock()
			adjustPrice(msg.Type)

		case "try-sell":
			data.TrySellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{
				Time: time.Now(), Type: "try-sell",
			})
			mutex.Unlock()
			updateTelegramMessage()

		case "info":
			err = ws.WriteJSON(PriceAndRatio{
				Prices: data.Prices,
				Ratios: data.Ratios,
			})
			mutex.Unlock()
			if err != nil {
				log.Printf("write error: %v", err)
				return
			}

		case "presence":
			clientItems[ws] = make(map[string]int)
			clientInventory[ws] = make(map[string]int)

			for item, count := range msg.Items {
				if count > 0 {
					clientItems[ws][item] = count
				}
			}
			for item, count := range msg.Inventory {
				if count > 0 {
					clientInventory[ws][item] = count
				}
			}
			mutex.Unlock()

		default:
			mutex.Unlock()
		}

		mutex.Lock()
		saveDailyData()
		mutex.Unlock()
	}
}

func fixPrice() {
	for {
		if getConnectedClientsCount() == 0 {
			log.Println("Нет подключенных клиентов")
		} else {
			log.Println("fixing all prices ", time.Now().Format("15:04:05"))
			items := []string{
				"sword5", "sword6", "sword7", "sword5-unbreak", "sword6-unbreak", "sword7-unbreak", "pochti-megasword", "elytra",
                "elytra-unbreak", "megasword",
			}
			for _, item := range items {
				adjustPrice(item)
			}
		}
		time.Sleep(1 * time.Minute)
	}
}

func countRecentSales(item string, since time.Time) int {
    if since.IsZero() {
        return 0
    }
    
    count := 0
    for _, trade := range data.TradeHistory[item] {
        // Добавляем небольшой допуск в 1 секунду для сравнения времени
        if trade.Type == "sell" && !trade.Time.Before(since.Add(-time.Second)) && trade.Time.Before(time.Now()) {
            count++
        }
    }
    return count
}

func getItemCount(item string) int {
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, items := range clientItems {
		count += items[item]
	}
	return count
}

func getInventoryCount(item string) int {
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, items := range clientInventory {
		count += items[item]
	}
	return count
}

func getInventoryFreeSlots(itemType string) int {
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, items := range clientInventory {
		for t, c := range items {
			if itemsConfig[t].Type == itemType {
				count += c
			}
		}
	}
	return inventoryLimit[itemType] - count
}

func countRecentBuys(item string, since time.Time) int {
    if since.IsZero() {
        return 0
    }
    
    count := 0
    for _, trade := range data.TradeHistory[item] {
        // Добавляем небольшой допуск в 1 секунду для сравнения времени
        if trade.Type == "buy" && !trade.Time.Before(since.Add(-time.Second)) && trade.Time.Before(time.Now()) {
            count++
        }
    }
    return count
}
func countRecentTrySells(item string, since time.Time) int {
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "try-sell" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func adjustPrice(item string) {
	cfg, ok := itemsConfig[item]
	if !ok {
		return
	}

	mutex.Lock()
	now := time.Now()

	lastUpdate, updatedBefore := swordTimes[item]
	    if updatedBefore {
        // Добавить проверку на разумный интервал
        if now.Sub(lastUpdate) > 24*time.Hour {
            lastUpdate = now.Add(-cfg.AnalysisTime)
        }
    }
	swordTimes[item] = now

	if !updatedBefore {
		lastUpdate = now.Add(-cfg.AnalysisTime)
	}

	sales := countRecentSales(item, lastUpdate)
	buys := countRecentBuys(item, lastUpdate)
	newPrice := data.Prices[item]
	priceBefore := newPrice
	ratioBefore := data.Ratios[item]

	salesRate := float64(cfg.NormalSales) / cfg.AnalysisTime.Minutes()
	totalSalesRate := 0.0

	for _, otherCfg := range itemsConfig {
		if otherCfg.Type == cfg.Type {
			totalSalesRate += float64(otherCfg.NormalSales) / otherCfg.AnalysisTime.Minutes()
		}
	}
	if totalSalesRate == 0 {
		totalSalesRate = 1
	}
	itemShare := salesRate / totalSalesRate

	maxSlots := itemLimit[cfg.Type]
	allocatedSlots := int(math.Round(itemShare * float64(maxSlots)))
	if allocatedSlots < 1 {
		allocatedSlots = 1
	}

	var (
		totalTypeItems   int
		currentItemCount int
		totalInventory   int
		inventoryCount    int
	)

	for _, items := range clientItems {
		for name, count := range items {
			if itemsConfig[name].Type == cfg.Type {
				totalTypeItems += count
			}
			if name == item {
				currentItemCount += count
			}
		}
	}
	for _, inv := range clientInventory {
		for name, count := range inv {
			if itemsConfig[name].Type == cfg.Type {
				totalInventory += count
			}
			if name == item {
				inventoryCount += count
			}
		}
	}

	inventoryFreeSlots := inventoryLimit[cfg.Type] - totalInventory
	freeSlots := maxSlots - (totalTypeItems - currentItemCount)

	ratio := ratioBefore
	if sales >= cfg.NormalSales {
		expectedBuys := float64(sales) + 1.5*math.Sqrt(float64(sales))
		exxpectedInventory := 2*math.Sqrt(float64(sales))
		if sales >= 3 && (float64(buys) > expectedBuys || float64(exxpectedInventory) < float64(inventoryCount)) {
			if ratio == 0.8 {
				ratio = 0.75
			}
		} else if (buys < cfg.NormalSales || inventoryCount < cfg.NormalSales) && inventoryFreeSlots > cfg.NormalSales {
			if ratio == 0.75 {
				ratio = 0.8
			} else {
				if freeSlots < allocatedSlots {
					mutex.Unlock()
					return
				}
				newPrice += cfg.PriceStep
				if newPrice > cfg.MaxPrice {
					newPrice = cfg.MaxPrice
				}
			}
		}
	} else {
		allowedStock := cfg.NormalSales
		if cfg.NormalSales <= 1 {
			allowedStock += 2
		} else if cfg.NormalSales <= 3 {
			allowedStock += 1
		}

		if currentItemCount > allowedStock{
			newPrice -= cfg.PriceStep
			ratio = 0.8
			if newPrice < cfg.MinPrice {
				newPrice = cfg.MinPrice
			}
		} else if inventoryFreeSlots > cfg.NormalSales && inventoryCount < cfg.NormalSales {
			if freeSlots < allocatedSlots && buys > cfg.NormalSales {
				mutex.Unlock()
				return
			}
			if ratio == 0.75 {
				ratio = 0.8
			} else {
				newPrice += cfg.PriceStep
				if newPrice > cfg.MaxPrice {
					newPrice = cfg.MaxPrice
				}
			}
		}
	}

	if newPrice != priceBefore || ratio != ratioBefore {
		data.Prices[item] = newPrice
		dailyData.Prices[item] = newPrice
		data.Ratios[item] = ratio
		dailyData.Ratios[item] = ratio
		lastPriceUpdate[item] = now
		mutex.Unlock()

		// Отправляем обновленные данные всем клиентам
		mutex.Lock()
		priceData := PriceAndRatio{
			Prices: data.Prices,
			Ratios: data.Ratios,
		}
		// Создаем копию clients для использования вне блокировки
		clientsCopy := make([]*websocket.Conn, 0, len(clients))
		for client := range clients {
			clientsCopy = append(clientsCopy, client)
		}
		mutex.Unlock()

		for _, client := range clientsCopy {
			_ = client.WriteJSON(priceData)
		}

		updateTelegramMessage()
	} else {
		mutex.Unlock()
	}
}

func startStatsSender() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mutex.Lock()
		now := time.Now()

		for item, cfg := range itemsConfig {
			lastUpdate, ok := swordTimes[item]
			if !ok {
				continue
			}

			sales := countRecentSales(item, lastUpdate)
			price := data.Prices[item]
			ratio := data.Ratios[item]

			mutex.Unlock()
			sendIntervalStatsToTelegram(
				item,
				lastUpdate,
				now,
				sales,
				cfg.NormalSales,
				price,
				ratio,
			)
			mutex.Lock()
		}
		mutex.Unlock()
	}
}

func updateTelegramMessage() {
	updateTelegramMessageSimple()
}

func sendIntervalStatsToTelegram(item string, start, end time.Time, actualSales, expectedSales, price int, ratio float64) {
	status := "✅"
	if actualSales < expectedSales {
		status = "⚠️"
	}

	// 1. Получаем онлайн с внешнего сервера
	onlineCount := getOnlineCount()

	// 2. Подсчитываем покупки за интервал
	mutex.Lock()
	buyCount := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(start) && trade.Time.Before(end) {
			buyCount++
		}
	}
	mutex.Unlock()

	// 3. Получаем количество предметов на руках у клиентов
	mutex.Lock()
	onHand := getItemCount(item)
	mutex.Unlock()

	// 4. Формируем сообщение
	msg := fmt.Sprintf(
		"*%s* %s\n"+
			"⏳ Интервал: %s - %s\n"+
			"📦 Покупки: *%d*\n"+
			"📊 Продажи: *%d* из *%d* (норма)\n"+
			"💸 Цена: %d\n"+
			"🧮 Коэффициент: %.2f\n"+
			"🎒 На ah: %d\n"+
			"🎒 В инвентаре: %d\n"+
			"👥 Онлайн: %d игроков",
		item,
		status,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		buyCount,
		actualSales,
		expectedSales,
		price,
		ratio,
		onHand,
		getInventoryCount(item),
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
        return // Просто логируем ошибку и выходим
    }
	// 6. Сохраняем лог в файл (без Markdown)
	plainLog := fmt.Sprintf(
		"%s [%s → %s] %s | Покупки: %d | Продажи: %d/%d | Цена: %d | На руках: %d | Онлайн: %d\n",
		item,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		status,
		buyCount,
		actualSales,
		expectedSales,
		price,
		onHand,
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