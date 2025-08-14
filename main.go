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
	Prices map[string]int
	Ratios map[string]float64
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
	"netherite_sword": 144,
	"elytra":          24,
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
	Date      string
	Prices    map[string]int
	Ratios    map[string]float64
	BuyStats  map[string]int
	SellStats map[string]int
	MessageID int
}

	type TradeLog struct {
	Time  time.Time
	Type  string // "buy" или "sell"
	}	

var (
	itemsConfig = map[string]ItemConfig{
		"sword5": {
			BasePrice:    2100001,
			NormalSales:  10,
			PriceStep:    100000,
			AnalysisTime: 10 * time.Minute,
			MinPrice:     500001,
			MaxPrice:     5000001,
			Type:         "netherite_sword",
		},
		// ... остальные предметы ...
	}



	data struct {
		Prices       map[string]int
		Ratios       map[string]float64
		BuyStats     map[string]int
		SellStats    map[string]int
		LastTrade    map[string]time.Time
		TradeHistory map[string][]TradeLog
	}
	dataMu sync.Mutex

	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex

	currentDay     string
	dailyData      DailyData
	swordTimes    = make(map[string]time.Time)
	swordTimesMu  sync.Mutex
	lastPriceUpdate = make(map[string]time.Time)
	lastPriceUpdateMu sync.Mutex
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

	dataMu.Lock()
	err = ws.WriteJSON(PriceAndRatio{
		Prices: data.Prices,
		Ratios: data.Ratios,
	})
	dataMu.Unlock()
	
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

		var msg struct {
			Action string         `json:"action"`
			Type   string         `json:"type"`
			Items  map[string]int `json:"items"`
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
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{
				Time: time.Now(),
				Type: "buy",
			})
			adjustPrice(msg.Type)

		case "sell":
			data.SellStats[msg.Type]++
			data.LastTrade[msg.Type] = time.Now()
			data.TradeHistory[msg.Type] = append(data.TradeHistory[msg.Type], TradeLog{
				Time: time.Now(),
				Type: "sell",
			})
			adjustPrice(msg.Type)

		case "info":
			err := ws.WriteJSON(PriceAndRatio{
				Prices: data.Prices,
				Ratios: data.Ratios,
			})
			if err != nil {
				log.Printf("write error: %v", err)
			}

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

	sales := countRecentSales(item, lastUpdate)
	buys := countRecentBuys(item, lastUpdate)
	newPrice := data.Prices[item]
	priceBefore := newPrice
	ratioBefore := data.Ratios[item]
	ratio := ratioBefore

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

	maxSlots, ok := itemLimit[cfg.Type]
	if !ok {
		maxSlots = 1
	}

	allocatedSlots := int(math.Round(itemShare * float64(maxSlots)))
	if allocatedSlots < 1 {
		allocatedSlots = 1
	}

	totalTypeItems := 0
	currentItemCount := 0

	clientItemsMu.Lock()
	for _, items := range clientItems {
		for itemName, count := range items {
			if itemsConfig[itemName].Type == cfg.Type {
				totalTypeItems += count
				if itemName == item {
					currentItemCount += count
				}
			}
		}
	}
	clientItemsMu.Unlock()

	freeSlots := maxSlots - (totalTypeItems - currentItemCount)

	if freeSlots < allocatedSlots {
		return
	}

	if sales >= cfg.NormalSales {
		expectedBuys := float64(sales) + 1.5*math.Sqrt(float64(sales))
		if sales >= 3 && float64(buys) > expectedBuys {
			if ratio == 0.8 {
				ratio = 0.7
			} else {
				newPrice -= cfg.PriceStep
				ratio = 0.8
				if newPrice < cfg.MinPrice {
					newPrice = cfg.MinPrice
				}
			}
		}
	} else {
		expectedStock := cfg.NormalSales
		if currentItemCount > expectedStock {
			newPrice -= cfg.PriceStep
			ratio = 0.8
			if newPrice < cfg.MinPrice {
				newPrice = cfg.MinPrice
			}
		} else {
			if ratio == 0.7 {
				ratio = 0.8
			} else {
				newPrice += cfg.PriceStep
				if newPrice > cfg.MaxPrice {
					newPrice = cfg.MaxPrice
				}
			}
		}
	}

	sendIntervalStatsToTelegram(
		item,
		lastUpdate,
		now,
		sales,
		cfg.NormalSales,
		priceBefore,
		newPrice,
	)

	if newPrice != data.Prices[item] || ratio != ratioBefore {
		data.Prices[item] = newPrice
		dailyData.Prices[item] = newPrice
		data.Ratios[item] = ratio
		dailyData.Ratios[item] = ratio
		
		lastPriceUpdateMu.Lock()
		lastPriceUpdate[item] = now
		lastPriceUpdateMu.Unlock()

		clientsMu.Lock()
		for client := range clients {
			err := client.WriteJSON(PriceAndRatio{
				Prices: data.Prices,
				Ratios: data.Ratios,
			})
			if err != nil {
				log.Printf("write error: %v", err)
			}
		}
		clientsMu.Unlock()

		updateTelegramMessage()
	}
}

func updateTelegramMessage() {
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	msgText := fmt.Sprintf("📊 Статистика за %s\nПоследнее обновление: %s\n\n", dailyData.Date, currentTime)

	dataMu.Lock()
	defer dataMu.Unlock()

	for item := range itemsConfig {
		msgText += fmt.Sprintf(
			"🔹 %s: %d ₽ (коэф. %.1f)\n🛒 Куплено: %d\n💰 Продано: %d\n\n",
			item,
			data.Prices[item],
			data.Ratios[item],
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

func getConnectedClientsCount() int {
    clientsMu.Lock()
    defer clientsMu.Unlock()
    return len(clients)
}