package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

func main() {
	port := flag.String("port", "8080", "服务端口")
	output := flag.String("output", "data/images", "图片输出目录")
	flag.Parse()

	srv := NewServer(*output, indexHTML)

	http.HandleFunc("/", srv.handlePage)
	http.HandleFunc("/download", srv.handleDownload)
	http.HandleFunc("/events", srv.handleEvents)

	log.Printf("服务启动: http://localhost:%s", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal(err)
	}
}
