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
	clientItemsMu   sync.RWMutex
)

var itemLimit = map[string]int{
	"netherite_sword": 140,
	"elytra":          24,
}

var inventoryLimit = map[string]int{
	"netherite_sword": 36 * 3 * 6,
	"elytra":          36 * 3,
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
			BasePrice:    2300001,
			NormalSales:  12,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500001,
			MaxPrice:     5000001,
			Type:         "netherite_sword",
		},
		"sword6": {
			BasePrice:    2600002,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600002,
			MaxPrice:     6000002,
			Type:         "netherite_sword",
		},
		"sword7": {
			BasePrice:    3800003,
			NormalSales:  10,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     700003,
			MaxPrice:     7000003,
			Type:         "netherite_sword",
		},
		"pochti-megasword": {
			BasePrice:    6000004,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1000004,
			MaxPrice:     8000004,
			Type:         "netherite_sword",
		},
		"megasword": {
			BasePrice:    6200005,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1200005,
			MaxPrice:     10000005,
			Type:         "netherite_sword",
		},
		"elytra": {
			BasePrice:    1300006,
			NormalSales:  11,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     200006,
			MaxPrice:     30000006,
			Type:         "elytra",
		},
		"elytra-mend": {
			BasePrice:    5200007,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 20 * time.Minute,
			MinPrice:     500007,
			MaxPrice:     8000007,
			Type:         "elytra",
		},
		"elytra-unbreak": {
			BasePrice:    2300008,
			NormalSales:  5,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     300008,
			MaxPrice:     5000008,
			Type:         "elytra",
		},
		"mend": {
			BasePrice:    4000009,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700009,
			MaxPrice:     5500009,
			Type:         "netherite_sword",
		},
		"poison1": {
			BasePrice:    4100010,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700010,
			MaxPrice:     7000010,
			Type:         "netherite_sword",
		},
		"poison2": {
			BasePrice:    5000011,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700011,
			MaxPrice:     7000011,
			Type:         "netherite_sword",
		},
		"poison3": {
			BasePrice:    5200012,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700012,
			MaxPrice:     7000012,
			Type:         "netherite_sword",
		},
		"vampiryzm1": {
			BasePrice:    5100013,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700013,
			MaxPrice:     7000013,
			Type:         "netherite_sword",
		},
		"vampiryzm2": {
			BasePrice:    5200014,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 13 * time.Minute,
			MinPrice:     700014,
			MaxPrice:     7000014,
			Type:         "netherite_sword",
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
	dataMu sync.RWMutex

	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.RWMutex

	currentDay string
	dailyData  DailyData

	swordTimes   = make(map[string]time.Time)
	swordTimesMu sync.RWMutex

	lastPriceUpdate   = make(map[string]time.Time)
	lastPriceUpdateMu sync.RWMutex
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
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	return len(clients)
}

func loadDailyData(loc *time.Location) {
	dataMu.Lock()

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
	swordTimesMu.Lock()
	for item := range itemsConfig {
		swordTimes[item] = time.Now()
	}
	swordTimesMu.Unlock()

	// Создаем копии данных для Telegram сообщения ДО разблокировки
	prices := make(map[string]int)
	buyStats := make(map[string]int)
	sellStats := make(map[string]int)
	for k, v := range data.Prices {
		prices[k] = v
	}
	for k, v := range data.BuyStats {
		buyStats[k] = v
	}
	for k, v := range data.SellStats {
		sellStats[k] = v
	}
	
	messageID := dailyData.MessageID
	date := dailyData.Date
	
	// Разблокируем мьютекс перед вызовом Telegram функции
	dataMu.Unlock()
	
	// Обновляем Telegram сообщение с уже скопированными данными
	// НЕ вызываем saveDailyData внутри этой функции
	updateTelegramMessageWithStatsNoSave(prices, buyStats, sellStats, date, messageID)
}

// Новая функция без сохранения данных
func updateTelegramMessageWithStatsNoSave(prices, buyStats, sellStats map[string]int, date string, messageID int) {
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

	var newMessageID int
	if messageID == 0 {
		msg, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   msgText,
		})
		if err != nil {
			log.Printf("[Telegram error] Не удалось отправить новое сообщение: %v", err)
			return
		}
		newMessageID = msg.ID
	} else {
		_, err := tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
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
				newMessageID = msg.ID
			} else {
				log.Printf("[Telegram error] Повторная отправка тоже не удалась: %v", sendErr)
				return
			}
		}
	}

	// Обновляем messageID если он изменился
	if newMessageID != 0 {
		dataMu.Lock()
		dailyData.MessageID = newMessageID
		// Сохраняем данные отдельно, без вызова updateTelegramMessage
		saveDailyDataNoUpdate()
		dataMu.Unlock()
	}
}

// Новая функция сохранения без обновления Telegram
func saveDailyDataNoUpdate() {
	// Эта функция вызывается уже с заблокированным dataMu
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

func checkDayChange(loc *time.Location) {
	for {
		now := time.Now().In(loc)
		nextDay := now.Add(24 * time.Hour)
		nextDay = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, loc)
		time.Sleep(time.Until(nextDay))

		// Новый день - сохраняем данные и создаем новое сообщение
		dataMu.Lock()
		saveDailyData()
		dataMu.Unlock()
		
		loadDailyData(loc) // Перезагружаем данные для нового дня
	}
}

func saveDailyData() {
	dataMu.RLock()
	defer dataMu.RUnlock()
	
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

	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()

	clientItemsMu.Lock()
	clientItems[ws] = make(map[string]int)
	clientInventory[ws] = make(map[string]int)
	clientItemsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, ws)
		clientsMu.Unlock()

		clientItemsMu.Lock()
		delete(clientItems, ws)
		delete(clientInventory, ws)
		clientItemsMu.Unlock()
	}()

	// Отправляем текущие цены при подключении
	dataMu.RLock()
	err = ws.WriteJSON(PriceAndRatio{
		Prices: data.Prices,
		Ratios: data.Ratios,
	})
	dataMu.RUnlock()
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
			Action    string            `json:"action"`
			Type      string            `json:"type"`   // для buy/sell
			Items     map[string]int    `json:"items"`  // для presence
			Inventory map[string]int    `json:"inventory"`
		}

		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("json unmarshal error: %v", err)
			continue
		}

		switch msg.Action {
		case "buy":
			dataMu.Lock()
			data.BuyStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "buy"})
			dataMu.Unlock()
			updateTelegramMessage()

		case "sell":
			dataMu.Lock()
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "sell"})
			dataMu.Unlock()
			adjustPrice(msg.Type)

		case "try-sell":
			dataMu.Lock()
			data.TrySellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{
				Time: time.Now(), Type: "try-sell",
			})
			dataMu.Unlock()
			updateTelegramMessage()

		case "info":
			dataMu.RLock()
			err = ws.WriteJSON(PriceAndRatio{
				Prices: data.Prices,
				Ratios: data.Ratios,
			})
			dataMu.RUnlock()
			if err != nil {
				log.Printf("write error: %v", err)
				return
			}

		case "presence":
			clientItemsMu.Lock()
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
			clientItemsMu.Unlock()
		}
		
		dataMu.Lock()
		saveDailyData()
		dataMu.Unlock()
	}
}

func fixPrice() {
	for {
		if getConnectedClientsCount() == 0 {
			log.Println("Нет подключенных клиентов")
		} else {
			log.Println("fixing all prices ", time.Now().Format("15:04:05"))
			items := []string{
				"sword5", "sword6", "sword7", "pochti-megasword", "elytra",
				"elytra-mend", "elytra-unbreak", "megasword", "mend", "poison1",
				"poison2", "poison3", "vampiryzm1", "vampiryzm2",
			}
			for _, item := range items {
				adjustPrice(item)
			}
		}
		time.Sleep(1 * time.Minute)
	}
}

func countRecentSales(item string, since time.Time) int {
	dataMu.RLock()
	defer dataMu.RUnlock()
	
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "sell" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func getItemCount(item string) int {
	clientItemsMu.RLock()
	defer clientItemsMu.RUnlock()

	count := 0
	for _, items := range clientItems {
		count += items[item]
	}
	return count
}

func getInventoryCount(item string) int {
	clientItemsMu.RLock()
	defer clientItemsMu.RUnlock()

	count := 0
	for _, items := range clientInventory {
		count += items[item]
	}
	return count
}

func getInventoryFreeSlots(itemType string) int {
	clientItemsMu.RLock()
	defer clientItemsMu.RUnlock()

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
	dataMu.RLock()
	defer dataMu.RUnlock()
	
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func countRecentTrySells(item string, since time.Time) int {
	dataMu.RLock()
	defer dataMu.RUnlock()
	
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

	now := time.Now()

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

	dataMu.RLock()
	sales := countRecentSales(item, lastUpdate)
	buys := countRecentBuys(item, lastUpdate)
	newPrice := data.Prices[item]
	priceBefore := newPrice
	ratioBefore := data.Ratios[item]
	dataMu.RUnlock()

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
	)

	clientItemsMu.RLock()
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
		}
	}
	clientItemsMu.RUnlock()

	inventoryFreeSlots := inventoryLimit[cfg.Type] - totalInventory
	freeSlots := maxSlots - (totalTypeItems - currentItemCount)

	ratio := ratioBefore
	if sales >= cfg.NormalSales {
		expectedBuys := float64(sales) + 1.5*math.Sqrt(float64(sales))
		if sales >= 3 && float64(buys) > expectedBuys {
			if ratio == 0.8 {
				ratio = 0.75
			}
		} else if buys < cfg.NormalSales && inventoryFreeSlots > cfg.NormalSales {
			if ratio == 0.75 {
				ratio = 0.8
			} else {
				if freeSlots < allocatedSlots {
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

		if currentItemCount > allowedStock {
			newPrice -= cfg.PriceStep
			ratio = 0.8
			if newPrice < cfg.MinPrice {
				newPrice = cfg.MinPrice
			}
		} else if inventoryFreeSlots > cfg.NormalSales {
			if freeSlots < allocatedSlots && !(buys < cfg.NormalSales) {
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
		dataMu.Lock()
		data.Prices[item] = newPrice
		dailyData.Prices[item] = newPrice
		data.Ratios[item] = ratio
		dailyData.Ratios[item] = ratio
		lastPriceUpdateMu.Lock()
		lastPriceUpdate[item] = now
		lastPriceUpdateMu.Unlock()
		dataMu.Unlock()

		// Отправляем обновленные данные всем клиентам
		dataMu.RLock()
		priceData := PriceAndRatio{
			Prices: data.Prices,
			Ratios: data.Ratios,
		}
		dataMu.RUnlock()

		clientsMu.RLock()
		for client := range clients {
			_ = client.WriteJSON(priceData)
		}
		clientsMu.RUnlock()

		updateTelegramMessage()
	}
}

func startStatsSender() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		dataMu.RLock()
		now := time.Now()

		for item, cfg := range itemsConfig {
			swordTimesMu.RLock()
			lastUpdate, ok := swordTimes[item]
			swordTimesMu.RUnlock()
			
			if !ok {
				continue
			}

			sales := countRecentSales(item, lastUpdate)
			dataMu.RUnlock()
			
			dataMu.RLock()
			price := data.Prices[item]
			ratio := data.Ratios[item]
			dataMu.RUnlock()

			sendIntervalStatsToTelegram(
				item,
				lastUpdate,
				now,
				sales,
				cfg.NormalSales,
				price,
				price,
				ratio,
			)
		}
		dataMu.RUnlock()
	}
}

func updateTelegramMessage() {
	dataMu.RLock()
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
	dataMu.RUnlock()
	
	updateTelegramMessageWithStats(prices, buyStats, sellStats, date, messageID)
}

func updateTelegramMessageWithStats(prices, buyStats, sellStats map[string]int, date string, messageID int) {
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

	if messageID == 0 {
		msg, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   msgText,
		})
		if err != nil {
			log.Printf("[Telegram error] Не удалось отправить новое сообщение: %v", err)
			return
		}
		dataMu.Lock()
		dailyData.MessageID = msg.ID
		saveDailyData()
		dataMu.Unlock()
	} else {
		_, err := tgBot.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: messageID,
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
				dataMu.Lock()
				dailyData.MessageID = msg.ID
				saveDailyData()
				dataMu.Unlock()
			} else {
				log.Printf("[Telegram error] Повторная отправка тоже не удалась: %v", sendErr)
			}
		}
	}
}

func sendIntervalStatsToTelegram(item string, start, end time.Time, actualSales, expectedSales, priceBefore, priceAfter int, ratio float64) {
	status := "✅"
	if actualSales < expectedSales {
		status = "⚠️"
	}

	// 1. Получаем онлайн с внешнего сервера
	onlineCount := getOnlineCount()

	// 2. Подсчитываем покупки за интервал
	dataMu.RLock()
	buyCount := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(start) && trade.Time.Before(end) {
			buyCount++
		}
	}
	dataMu.RUnlock()

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
		ratio,
		onHand,
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