package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	port := flag.String("port", "8081", "listen port")
	flag.Parse()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "backend :%s | path=%s | X-Forwarded-For=%s\n",
			*port, r.URL.Path, r.Header.Get("X-Forwarded-For"))
	})

	log.Printf("test backend on :%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
