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
	"netherite_sword": 24*3,
	// "elytra":          24,
	// "gunpowder":       8,
	// "netherite_chestplate": 24,
}

var inventoryLimit = map[string]int{
	"netherite_sword": 28*3*3,
	// "elytra":          28 * 3,
	// "gunpowder":       28,
	// "netherite_chestplate": 28*3,
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
		"sword6": {
			BasePrice:    1500002,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600002,
			MaxPrice:     6000002,
			Type:         "netherite_sword",
		},
		"sword7": {
			BasePrice:    2000003,
			NormalSales:  8,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     700003,
			MaxPrice:     7000003,
			Type:         "netherite_sword",
		},
		"sword5-unbreak": {
			BasePrice:    1000004,
			NormalSales:  10,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500004,
			MaxPrice:     5000004,
			Type:         "netherite_sword",
		},
		"sword6-unbreak": {
			BasePrice:    1600005,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     600005,
			MaxPrice:     6000005,
			Type:         "netherite_sword",
		},
		"pochti-megasword": {
			BasePrice:    2800007,
			NormalSales:  1,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1000007,
			MaxPrice:     8000007,
			Type:         "netherite_sword",
		},
		"megasword": {
			BasePrice:    3300008,
			NormalSales:  2,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     1200008,
			MaxPrice:     10000008,
			Type:         "netherite_sword",
		},
		// "elytra": {
		// 	BasePrice:    1000009,
		// 	NormalSales:  11,
		// 	PriceStep:    100000,
		// 	AnalysisTime: 10 * time.Minute,
		// 	MinPrice:     200009,
		// 	MaxPrice:     30000009,
		// 	Type:         "elytra",
		// },
		// "elytra-unbreak": {
		// 	BasePrice:    1500010,
		// 	NormalSales:  5,
		// 	PriceStep:    100000,
		// 	AnalysisTime: 10 * time.Minute,
		// 	MinPrice:     300010,
		// 	MaxPrice:     5000010,
		// 	Type:         "elytra",
		// },
		// "порох": {
		// 	BasePrice:    800011,
		// 	NormalSales:  5,
		// 	PriceStep:    100000,
		// 	AnalysisTime: 10 * time.Minute,
		// 	MinPrice:     600002,
		// 	MaxPrice:     6000002,
		// 	Type:         "gunpowder",
		// },
		// "нагрудник": {
		// 	BasePrice:    500011,
		// 	NormalSales:  5,
		// 	PriceStep:    100000,
		// 	AnalysisTime: 10 * time.Minute,
		// 	MinPrice:     600002,
		// 	MaxPrice:     6000002,
		// 	Type:         "gunpowder",
		// },
		// "нагрудник2": {
		// 	BasePrice:    1000011,
		// 	NormalSales:  5,
		// 	PriceStep:    100000,
		// 	AnalysisTime: 10 * time.Minute,
		// 	MinPrice:     600002,
		// 	MaxPrice:     6000002,
		// 	Type:         "gunpowder",
		// },
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

	// WebSocket сервер
	http.HandleFunc("/ws", handleConnections)
	go func() {
		log.Println("Server started on :8080")
		log.Print(http.ListenAndServe(":8080", nil))
	}()

	// Проверка смены дня
	go checkDayChange(loc)

	// Запускаем таймеры для каждого предмета
	go startItemTimers()

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
		// Устанавливаем время последнего обновления на начало AnalysisTime периода
		swordTimes[item] = time.Now().Add(-itemsConfig[item].AnalysisTime)
	}

	// Сохраняем данные
	saveDailyDataNoMessageUpdate()
}

func startItemTimers() {
	for item, cfg := range itemsConfig {
		go func(item string, cfg ItemConfig) {
			log.Printf("[TIMER] Запущен таймер для %s (интервал: %v)", item, cfg.AnalysisTime)
			
			// Первый запуск через короткую задержку, чтобы не все предметы начали одновременно
			time.Sleep(time.Duration(len(itemsConfig)-1) * time.Second)
			
			// Создаем тикер с интервалом AnalysisTime
			ticker := time.NewTicker(cfg.AnalysisTime)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					adjustAndReport(item, cfg)
				}
			}
		}(item, cfg)
	}
}

func getItemStatsForReporting(item string, since time.Time) (sales, buys, trySells, price int, ratio float64) {
	mutex.Lock()
	defer mutex.Unlock()
	
	sales = countRecentSales(item, since)
	buys = countRecentBuys(item, since)
	trySells = countRecentTrySells(item, since)
	price = data.Prices[item]
	ratio = data.Ratios[item]
	
	return
}

func getInventoryStats(item string) (onHand, inInventory int) {
	mutex.Lock()
	defer mutex.Unlock()
	
	onHand = getItemCount(item)
	inInventory = getInventoryCount(item)
	
	return
}

func adjustAndReport(item string, cfg ItemConfig) {
	// Определяем период анализа
	now := time.Now()
	start := now.Add(-cfg.AnalysisTime)
	
	// Получаем статистику за предыдущий период
	sales, buys, trySells, price, ratio := getItemStatsForReporting(item, start)
	
	// Логируем начало анализа
	log.Printf("[ANALYSIS] %s: анализ с %s по %s. Продажи: %d (норма: %d)", 
		item, start.Format("15:04:05"), now.Format("15:04:05"), sales, cfg.NormalSales)
	
	// Обновляем цену на основе статистики
	adjustPrice(item)
	
	// Получаем текущие данные после обновления
	newPrice, newRatio := func() (int, float64) {
		mutex.Lock()
		defer mutex.Unlock()
		return data.Prices[item], data.Ratios[item]
	}()
	
	// Отправляем статистику за ПРЕДЫДУЩИЙ период
	sendIntervalStatsToTelegram(
		item,
		start, now,
		float64(sales), float64(cfg.NormalSales), float64(buys), float64(trySells),
		float64(price), ratio, float64(newPrice), newRatio,
	)
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

			// Попробуем отправить заново, если редактирование не удалось
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
		mutex.Lock()
		dailyData.MessageID = newMessageID
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
			// НЕ ОБНОВЛЯЕМ ЦЕНУ - только по таймеру

		case "sell":
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{Time: time.Now(), Type: "sell"})
			mutex.Unlock()
			// НЕ ОБНОВЛЯЕМ ЦЕНУ - только по таймеру

		case "try-sell":
			data.TrySellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{
				Time: time.Now(), Type: "try-sell",
			})
			mutex.Unlock()
			// НЕ ОБНОВЛЯЕМ ЦЕНУ - только по таймеру

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

func countRecentSales(item string, since time.Time) int {
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "sell" && trade.Time.After(since) {
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
	// Эта функция вызывается с уже заблокированным mutex
	count := 0
	for _, trade := range data.TradeHistory[item] {
		if trade.Type == "buy" && trade.Time.After(since) {
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

	// Убираем проверку времени - она конфликтует с таймером
	// lastUpdate, updatedBefore := swordTimes[item]
	// if updatedBefore && now.Sub(lastUpdate) < cfg.AnalysisTime {
	//     mutex.Unlock()
	//     return
	// }
	
	// Всегда обновляем время последнего анализа
	swordTimes[item] = now

	// Берем начало периода как AnalysisTime назад
	lastUpdate := now.Add(-cfg.AnalysisTime)

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
		inventoryCount   int
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
		expectedInventory := 2*math.Sqrt(float64(sales))
		if sales >= 3 && (float64(buys) > expectedBuys || float64(expectedInventory) < float64(inventoryCount)) {
			if ratio == 0.8 {
				ratio = 0.75
			}
		} else if (buys < cfg.NormalSales) && inventoryFreeSlots > cfg.NormalSales {
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

		if currentItemCount > allowedStock {
			newPrice -= cfg.PriceStep
			if newPrice < cfg.MinPrice {
				newPrice = cfg.MinPrice
			}
		} else if inventoryFreeSlots > cfg.NormalSales {
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
	if item == "порох" {
		ratio = 0.7
	}

	if newPrice != priceBefore || ratio != ratioBefore {
		// Сохраняем изменения
		data.Prices[item] = newPrice
		dailyData.Prices[item] = newPrice
		data.Ratios[item] = ratio
		dailyData.Ratios[item] = ratio
		lastPriceUpdate[item] = now
		mutex.Unlock()

		log.Printf("[PRICE] %s: цена изменена с %d на %d", item, priceBefore, newPrice)

		// Отправляем обновленные данные всем клиентам
		sendPriceUpdateToClients()
	} else {
		mutex.Unlock()
	}
}

func sendPriceUpdateToClients() {
	// Создаем копию данных для отправки
	var priceData PriceAndRatio
	
	mutex.Lock()
	priceData = PriceAndRatio{
		Prices: make(map[string]int),
		Ratios: make(map[string]float64),
	}
	for k, v := range data.Prices {
		priceData.Prices[k] = v
	}
	for k, v := range data.Ratios {
		priceData.Ratios[k] = v
	}
	mutex.Unlock()

	// Отправляем клиентам
	mutex.Lock()
	clientsCopy := make([]*websocket.Conn, 0, len(clients))
	for client := range clients {
		clientsCopy = append(clientsCopy, client)
	}
	mutex.Unlock()

	for _, client := range clientsCopy {
		if err := client.WriteJSON(priceData); err != nil {
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

	// Получаем онлайн с внешнего сервера
	onlineCount := getOnlineCount()

	// Получаем количество предметов на руках у клиентов
	onHand, inInventory := getInventoryStats(item)

	// Формируем сообщение
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

	// Отправляем в Telegram
	ctx := context.Background()
	_, err := tgBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    -4633184325,
		Text:      msg,
		ParseMode: "Markdown",
	})
	if err != nil {
		log.Printf("[Telegram] Ошибка при отправке интервал-статы: %v", err)
	}

	// Сохраняем лог в файл
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