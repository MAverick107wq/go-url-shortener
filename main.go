package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
)

// Our "database" - lives in memory, but is backed up to urls.json on disk.
var urlStore = make(map[string]string)

// A mutex to prevent race conditions if multiple requests
// try to read/write to the map at the exact same time.
var mu sync.Mutex

const storeFile = "urls.json"
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateShortCode creates a random 6-character alphanumeric string.
func generateShortCode() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// loadURLStore reads urls.json from disk into urlStore on startup.
// If the file doesn't exist yet, it just starts with an empty map.
func loadURLStore() {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(storeFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No existing urls.json found. Starting fresh.")
			return
		}
		log.Println("Error reading urls.json:", err)
		return
	}

	if len(data) == 0 {
		return
	}

	if err := json.Unmarshal(data, &urlStore); err != nil {
		log.Println("Error parsing urls.json:", err)
		return
	}

	log.Printf("Loaded %d URLs from urls.json\n", len(urlStore))
}

// saveURLStoreToFile writes the current urlStore to disk as JSON.
// IMPORTANT: The caller must already hold the mutex lock before calling this.
func saveURLStoreToFile() error {
	data, err := json.MarshalIndent(urlStore, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storeFile, data, 0644)
}

// ShortenRequest models the incoming JSON body: {"long_url": "..."}
type ShortenRequest struct {
	LongURL string `json:"long_url"`
}

// ShortenResponse models the outgoing JSON body: {"short_code": "..."}
type ShortenResponse struct {
	ShortCode string `json:"short_code"`
}

func shortenHandler(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.LongURL == "" {
		http.Error(w, "long_url field is required", http.StatusBadRequest)
		return
	}

	shortCode := generateShortCode()

	mu.Lock()
	urlStore[shortCode] = req.LongURL
	if err := saveURLStoreToFile(); err != nil {
		log.Println("Error saving urls.json:", err)
		// Note: we don't fail the request here - the URL still works
		// in memory even if the disk write failed.
	}
	mu.Unlock()

	resp := ShortenResponse{ShortCode: shortCode}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// redirectHandler looks up the shortCode and redirects to the original URL.
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	shortCode := r.PathValue("shortCode")

	mu.Lock()
	longURL, found := urlStore[shortCode]
	mu.Unlock()

	if !found {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, longURL, http.StatusFound)
}

func main() {
	loadURLStore()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", helloHandler)
	mux.HandleFunc("POST /shorten", shortenHandler)
	mux.HandleFunc("GET /{shortCode}", redirectHandler)

	fmt.Println("Server starting on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, URL Shortener")
}