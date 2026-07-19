package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	outputBase string
	html       []byte

	logSubs     []chan string
	logMu       sync.RWMutex
	downloading bool
	dlMu        sync.Mutex
}

func NewServer(output string, html []byte) *Server {
	s := &Server{outputBase: output, html: html}
	if err := os.MkdirAll(output, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}
	return s
}

func (s *Server) addLog(msg string) {
	msg = "[" + time.Now().Format("15:04:05") + "] " + msg
	s.logMu.Lock()
	for _, ch := range s.logSubs {
		select {
		case ch <- msg:
		default:
		}
	}
	s.logMu.Unlock()
	fmt.Println(msg)
}

func (s *Server) notifyTotal(n int) {
	s.logMu.Lock()
	for _, ch := range s.logSubs {
		select {
		case ch <- "__TOTAL__:" + fmt.Sprintf("%d", n):
		default:
		}
	}
	s.logMu.Unlock()
}

func (s *Server) notifyProgress(total, current int) {
	s.logMu.Lock()
	for _, ch := range s.logSubs {
		select {
		case ch <- fmt.Sprintf("__PROGRESS__:%d/%d", current, total):
		default:
		}
	}
	s.logMu.Unlock()
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.html)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	s.dlMu.Lock()
	if s.downloading {
		s.dlMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "busy"})
		return
	}
	s.downloading = true
	s.dlMu.Unlock()

	mode := r.URL.Query().Get("mode")
	page := r.URL.Query().Get("page")
	size := r.URL.Query().Get("size")
	if mode == "auto" {
		page = "1"
		size = "50"
	}

	go func() {
		defer func() {
			s.dlMu.Lock()
			s.downloading = false
			s.dlMu.Unlock()
		}()

		if mode == "auto" {
			p := 1
			for {
				s.addLog(fmt.Sprintf("--- 翻页: page=%d ---", p))
				done := make(chan int, 1)
				go func(pageNum int) {
					done <- RunDownload(fmt.Sprintf("%d", pageNum), size, s.outputBase, s.addLog, s.notifyTotal, s.notifyProgress)
				}(p)
				count := <-done
				if count == 0 {
					s.addLog("已到最后一页，下载结束")
					break
				}
				p++
				time.Sleep(2 * time.Second)
			}
		} else {
			RunDownload(page, size, s.outputBase, s.addLog, s.notifyTotal, s.notifyProgress)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *Server) removeSub(ch chan string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	for i, sub := range s.logSubs {
		if sub == ch {
			s.logSubs = append(s.logSubs[:i], s.logSubs[i+1:]...)
			return
		}
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan string, 200)
	s.logMu.Lock()
	s.logSubs = append(s.logSubs, ch)
	s.logMu.Unlock()

	notify := r.Context().Done()
	go func() {
		<-notify
		s.removeSub(ch)
		close(ch)
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", escapeHTML(msg))
			flusher.Flush()
		case <-notify:
			return
		}
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&")
	s = strings.ReplaceAll(s, "<", "<")
	s = strings.ReplaceAll(s, ">", ">")
	return s
}
