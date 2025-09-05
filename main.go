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
	chatID   = -4709535234
	timezone = "Asia/Tashkent"
)

type PriceAndRatio struct {
	Prices map[string]int     `json:"prices"`
	Ratios map[string]float64 `json:"ratios"`
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

type TradeLog struct {
	Time time.Time
	Type string
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

type ItemConfig struct {
	BasePrice    int
	NormalSales  int
	PriceStep    int
	AnalysisTime time.Duration
	MinPrice     int
	MaxPrice     int
	Type         string
}

type ClientData struct {
	Items     map[string]int
	Inventory map[string]int
	Mutex     sync.Mutex
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	tgBot *bot.Bot

	dataMutex    sync.RWMutex
	clientsMutex sync.RWMutex
	dailyMutex   sync.RWMutex

	data = &Data{
		Prices:       make(map[string]int),
		Ratios:       make(map[string]float64),
		BuyStats:     make(map[string]int),
		SellStats:    make(map[string]int),
		TrySellStats: make(map[string]int),
		LastTrade:    make(map[string]time.Time),
		TradeHistory: make(map[string][]TradeLog),
	}

	clients = make(map[*websocket.Conn]*ClientData)

	itemsConfig = map[string]ItemConfig{
		"sword6": {
			BasePrice:    3000002,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600002,
			MaxPrice:     6000002,
			Type:         "netherite_sword",
		},
		"sword7": {
			BasePrice:    5000003,
			NormalSales:  8,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     700003,
			MaxPrice:     7000003,
			Type:         "netherite_sword",
		},
		"sword5-unbreak": {
			BasePrice:    2600004,
			NormalSales:  10,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500004,
			MaxPrice:     5000004,
			Type:         "netherite_sword",
		},
		"megasword": {
			BasePrice:    6200008,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1200008,
			MaxPrice:     10000008,
			Type:         "netherite_sword",
		},
	}

	swordTimes      = make(map[string]time.Time)
	currentDay      string
	dailyData       DailyData
	lastPriceUpdate = make(map[string]time.Time)
)

func main() {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("Error loading location: %v", err)
		os.Exit(1)
	}

	b, err := bot.New(token)
	if err != nil {
		log.Printf("Error creating bot: %v", err)
		os.Exit(1)
	}
	tgBot = b

	ctx := context.Background()
	_, err = tgBot.GetMe(ctx)
	if err != nil {
		log.Printf("Error checking bot: %v", err)
		os.Exit(1)
	}
	log.Println("Bot initialized successfully")

	loadDailyData(loc)

	http.HandleFunc("/ws", handleConnections)
	go func() {
		log.Println("Server started on :8080")
		log.Print(http.ListenAndServe(":8080", nil))
	}()

	go checkDayChange(loc)
	go startItemTimers()

	select {}
}

func loadDailyData(loc *time.Location) {
	dailyMutex.Lock()
	defer dailyMutex.Unlock()
	dataMutex.Lock()
	defer dataMutex.Unlock()

	today := time.Now().In(loc).Format("2006-01-02")
	currentDay = today
	filename := fmt.Sprintf("data_%s.json", today)

	dailyData = DailyData{
		Date:         today,
		Prices:       make(map[string]int),
		BuyStats:     make(map[string]int),
		SellStats:    make(map[string]int),
		TrySellStats: make(map[string]int),
		Ratios:       make(map[string]float64),
	}

	if file, err := os.ReadFile(filename); err == nil {
		if err := json.Unmarshal(file, &dailyData); err == nil && dailyData.Date == today {
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

	for item := range itemsConfig {
		swordTimes[item] = time.Now().Add(-itemsConfig[item].AnalysisTime)
	}

	saveDailyDataNoMessageUpdate()
}

func startItemTimers() {
	for item, cfg := range itemsConfig {
		go func(item string, cfg ItemConfig) {
			log.Printf("[TIMER] Запущен таймер для %s (интервал: %v)", item, cfg.AnalysisTime)
			
			time.Sleep(time.Duration(len(itemsConfig)-1) * time.Second)
			
			ticker := time.NewTicker(cfg.AnalysisTime)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					startTime := time.Now().Add(-cfg.AnalysisTime)
					
					// Получаем статистику за предыдущий период
					sales, buys, trySells, oldPrice, oldRatio := getItemStatsForReporting(item, startTime)
					onHand, inInventory := getItemAndInventoryCount(item)
					
					// Логируем начало анализа
					log.Printf("[ANALYSIS] %s: анализ с %s по %s. Продажи: %d (норма: %d)", 
						item, startTime.Format("15:04:05"), time.Now().Format("15:04:05"), sales, cfg.NormalSales)
					
					// Обновляем цену
					adjustPrice(item, onHand, inInventory)
					
					// Получаем новые данные после обновления
					newPrice, newRatio := getCurrentPriceAndRatio(item)
					
					// Отправляем статистику за предыдущий период
					go sendIntervalStatsToTelegram(
						item,
						startTime, time.Now(),
						float64(sales), float64(cfg.NormalSales), float64(buys), float64(trySells),
						float64(oldPrice), oldRatio, float64(newPrice), newRatio,
					)
				}
			}
		}(item, cfg)
	}
}

func getCurrentPriceAndRatio(item string) (int, float64) {
	dataMutex.RLock()
	defer dataMutex.RUnlock()
	return data.Prices[item], data.Ratios[item]
}

func getItemAndInventoryCount(item string) (int, int) {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	onHand := 0
	inInventory := 0
	for _, clientData := range clients {
		clientData.Mutex.Lock()
		onHand += clientData.Items[item]
		inInventory += clientData.Inventory[item]
		clientData.Mutex.Unlock()
	}
	return onHand, inInventory
}

func adjustPrice(item string, onHand, inInventory int) {
	cfg, ok := itemsConfig[item]
	if !ok {
		return
	}

	dataMutex.Lock()
	defer dataMutex.Unlock()

	now := time.Now()
	lastUpdate := now.Add(-cfg.AnalysisTime)
	swordTimes[item] = now

	sales := countRecentSalesLocked(item, lastUpdate)
	buys := countRecentBuysLocked(item, lastUpdate)
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

	maxSlots := 24 * 3
	allocatedSlots := int(math.Round(itemShare * float64(maxSlots)))
	if allocatedSlots < 1 {
		allocatedSlots = 1
	}

	inventoryLimit := 28 * 3 * 3
	inventoryFreeSlots := inventoryLimit - inInventory
	freeSlots := maxSlots - (onHand)

	ratio := ratioBefore
	if sales >= cfg.NormalSales {
		expectedBuys := float64(sales) + 1.5*math.Sqrt(float64(sales))
		expectedInventory := 2 * math.Sqrt(float64(sales))
		if sales >= 3 && (float64(buys) > expectedBuys || float64(expectedInventory) < float64(inInventory)) {
			if ratio == 0.8 {
				ratio = 0.75
			}
		} else if (buys < cfg.NormalSales) && inventoryFreeSlots > cfg.NormalSales {
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

		if onHand > allowedStock {
			newPrice -= cfg.PriceStep
			if newPrice < cfg.MinPrice {
				newPrice = cfg.MinPrice
			}
		} else if inventoryFreeSlots > cfg.NormalSales {
			if freeSlots < allocatedSlots {
				return
			}
			if ratio == 0.75 {
				ratio = 0.8
			} else if buys < cfg.NormalSales {
				newPrice += cfg.PriceStep
				if newPrice > cfg.MaxPrice {
					newPrice = cfg.MaxPrice
				}
			}
		}
	}

	if newPrice != priceBefore || ratio != ratioBefore {
		data.Prices[item] = newPrice
		data.Ratios[item] = ratio
		lastPriceUpdate[item] = now

		go func() {
			dailyMutex.Lock()
			dailyData.Prices[item] = newPrice
			dailyData.Ratios[item] = ratio
			saveDailyDataNoMessageUpdate()
			dailyMutex.Unlock()
		}()

		sendPriceUpdateToClients()
		log.Printf("[PRICE] %s: цена изменена с %d на %d", item, priceBefore, newPrice)
		
		go sendPriceChangeNotification(item, priceBefore, newPrice, ratioBefore, ratio)
	}
}

func sendPriceChangeNotification(item string, oldPrice, newPrice int, oldRatio, newRatio float64) {
	ctx := context.Background()
	message := fmt.Sprintf(
		"📈 Изменение цены\n\n🔹 %s\n💰 Цена: %d → %d\n🧮 Коэффициент: %.2f → %.2f\n⏰ Время: %s",
		item, oldPrice, newPrice, oldRatio, newRatio, time.Now().Format("2006-01-02 15:04:05"),
	)
	
	_, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      message,
	})
	if err != nil {
		log.Printf("[Telegram] Ошибка при отправке уведомления об изменении цены: %v", err)
	}
}

func countRecentSalesLocked(item string, since time.Time) int {
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "sell" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func countRecentBuysLocked(item string, since time.Time) int {
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func countRecentTrySellsLocked(item string, since time.Time) int {
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "try-sell" && trade.Time.After(since) {
			count++
		}
	}
	return count
}

func getItemStatsForReporting(item string, since time.Time) (int, int, int, int, float64) {
	dataMutex.RLock()
	defer dataMutex.RUnlock()

	sales := countRecentSalesLocked(item, since)
	buys := countRecentBuysLocked(item, since)
	trySells := countRecentTrySellsLocked(item, since)
	price := data.Prices[item]
	ratio := data.Ratios[item]

	return sales, buys, trySells, price, ratio
}

func saveDailyDataNoMessageUpdate() {
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

func updateTelegramMessageSimple() {
	dataMutex.RLock()
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
	dataMutex.RUnlock()

	dailyMutex.RLock()
	date := dailyData.Date
	messageID := dailyData.MessageID
	dailyMutex.RUnlock()

	updateTelegramMessageWithoutLocks(prices, buyStats, sellStats, date, messageID)
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

	if newMessageID != 0 {
		dailyMutex.Lock()
		dailyData.MessageID = newMessageID
		saveDailyDataNoMessageUpdate()
		dailyMutex.Unlock()
	}
}

func checkDayChange(loc *time.Location) {
	for {
		now := time.Now().In(loc)
		nextDay := now.Add(24 * time.Hour)
		nextDay = time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, loc)
		time.Sleep(time.Until(nextDay))

		dailyMutex.Lock()
		saveDailyDataNoMessageUpdate()
		dailyMutex.Unlock()

		loadDailyData(loc)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print(err, " upgrade error")
		return
	}
	defer ws.Close()

	clientData := &ClientData{
		Items:     make(map[string]int),
		Inventory: make(map[string]int),
	}

	clientsMutex.Lock()
	clients[ws] = clientData
	clientsMutex.Unlock()

	defer func() {
		clientsMutex.Lock()
		delete(clients, ws)
		clientsMutex.Unlock()
	}()

	dataMutex.RLock()
	err = ws.WriteJSON(PriceAndRatio{
		Prices: data.Prices,
		Ratios: data.Ratios,
	})
	dataMutex.RUnlock()
	
	if err != nil {
		log.Printf("write error: %v", err)
		return
	}

	for {
		_, rawMsg, err := ws.ReadMessage()
		if err != nil {
			log.Printf("read error: %v", err)
			break
		}

		log.Printf("[WS incoming] %s", string(rawMsg))

		var msg struct {
			Action    string         `json:"action"`
			Type      string         `json:"type"`
			Items     map[string]int `json:"items"`
			Inventory map[string]int `json:"inventory"`
		}

		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("json unmarshal error: %v", err)
			continue
		}

		switch msg.Action {
		case "buy":
			dataMutex.Lock()
			data.BuyStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "buy"})
			dataMutex.Unlock()

			go func() {
				dailyMutex.Lock()
				saveDailyDataNoMessageUpdate()
				dailyMutex.Unlock()
			}()

		case "sell":
			dataMutex.Lock()
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "sell"})
			dataMutex.Unlock()

			go func() {
				dailyMutex.Lock()
				saveDailyDataNoMessageUpdate()
				dailyMutex.Unlock()
			}()

		case "try-sell":
			dataMutex.Lock()
			data.TrySellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "try-sell"})
			dataMutex.Unlock()

			go func() {
				dailyMutex.Lock()
				saveDailyDataNoMessageUpdate()
				dailyMutex.Unlock()
			}()

		case "info":
			dataMutex.RLock()
			err = ws.WriteJSON(PriceAndRatio{
				Prices: data.Prices,
				Ratios: data.Ratios,
			})
			dataMutex.RUnlock()
			
			if err != nil {
				log.Printf("write error: %v", err)
				return
			}

		case "presence":
			clientsMutex.Lock()
			clientData.Mutex.Lock()
			clientData.Items = msg.Items
			clientData.Inventory = msg.Inventory
			clientData.Mutex.Unlock()
			clientsMutex.Unlock()
		}
	}
}

func sendPriceUpdateToClients() {
	dataMutex.RLock()
	priceData := PriceAndRatio{
		Prices: make(map[string]int),
		Ratios: make(map[string]float64),
	}
	for k, v := range data.Prices {
		priceData.Prices[k] = v
	}
	for k, v := range data.Ratios {
		priceData.Ratios[k] = v
	}
	dataMutex.RUnlock()

	clientsMutex.RLock()
	clientsCopy := make([]*websocket.Conn, 0, len(clients))
	for client := range clients {
		clientsCopy = append(clientsCopy, client)
	}
	clientsMutex.RUnlock()

	for _, client := range clientsCopy {
		clientsMutex.RLock()
		clientData, exists := clients[client]
		clientsMutex.RUnlock()
		
		if !exists {
			continue
		}

		clientData.Mutex.Lock()
		err := client.WriteJSON(priceData)
		clientData.Mutex.Unlock()
		
		if err != nil {
			log.Printf("Ошибка отправки обновления клиенту: %v", err)
		}
	}
}

func sendIntervalStatsToTelegram(item string, start, end time.Time, actualSales, expectedSales, buyCount, trySellCount, 
                                oldPrice, oldRatio, newPrice, newRatio float64) {
	status := "✅"
	if actualSales < expectedSales {
		status = "⚠️"
	}

	onlineCount := getOnlineCount()
	onHand, inInventory := getItemAndInventoryCount(item)

	msg := fmt.Sprintf(
		"*%s* %s\n"+
			"⏳ Интервал: %s - %s\n"+
			"📦 Покупки: *%.0f*\n"+
			"🛒 Попытки продаж: *%.0f*\n"+
			"📊 Продажи: *%.0f* из *%.0f* (норма)\n"+
			"💰 Цена: %d → %d (%s)\n"+
			"🧮 Коэффициент: %.2f → %.2f\n"+
			"🎒 На аукционе: %d\n"+
			"🎒 В инвентаре: %d\n"+
			"👥 Онлайн: %d игроков",
		item,
		status,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		buyCount,
		trySellCount,
		actualSales,
		expectedSales,
		int(oldPrice), int(newPrice), 
		getPriceChangeEmoji(int(oldPrice), int(newPrice)),
		oldRatio, newRatio,
		onHand,
		inInventory,
		onlineCount,
	)

	ctx := context.Background()
	_, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    -4633184325,
		Text:      msg,
		ParseMode: "Markdown",
	})
	if err != nil {
		log.Printf("[Telegram] Ошибка при отправке интервал-статы: %v", err)
	}

	plainLog := fmt.Sprintf(
		"%s [%s → %s] %s | Покупки: %.0f | Продажи: %.0f/%.0f | Цена: %d→%d | Коэф: %.2f→%.2f | На руках: %d | Онлайн: %d\n",
		item,
		start.Format("15:04:05"),
		end.Format("15:04:05"),
		status,
		buyCount,
		actualSales,
		expectedSales,
		int(oldPrice), int(newPrice),
		oldRatio, newRatio,
		onHand,
		onlineCount,
	)

	appendToFile("logs_interval.txt", plainLog)
}

func getPriceChangeEmoji(oldPrice, newPrice int) string {
	if newPrice > oldPrice {
		return "📈 +"
	} else if newPrice < oldPrice {
		return "📉 -"
	}
	return "↔️ ="
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