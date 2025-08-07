package main

import (
	"fmt"
	"log"
	"net/http"
)

func downloads(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "downloading...\n")
}

func main() {
	http.HandleFunc("/downloads", downloads)

	log.Println("Starting Torrus API on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}

}
