package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

const dataFile = "counters.json"

var (
	mutex sync.Mutex
)

// Структура для хранения счетчиков
type Counters map[string]int

func main() {
	http.HandleFunc("/update", updateCountersHandler)
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Обработчик POST-запросов
func updateCountersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
		return
	}

	var keys []string
	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Обновляем счетчики
	counters, err := updateCounters(keys)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error updating counters: %v", err), http.StatusInternalServerError)
		return
	}

	// Возвращаем обновленные счетчики
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counters)
}

// Обновляет счетчики в JSON-файле
func updateCounters(keys []string) (Counters, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Чтение текущих данных
	counters := make(Counters)
	data, err := ioutil.ReadFile(dataFile)
	if err == nil {
		if err := json.Unmarshal(data, &counters); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %v", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Обновление счетчиков
	for _, key := range keys {
		counters[key]++
	}

	// Сохранение в файл
	newData, err := json.MarshalIndent(counters, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	if err := ioutil.WriteFile(dataFile, newData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %v", err)
	}

	return counters, nil
}