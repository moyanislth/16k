package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const pageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>16K 图片下载工具</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            min-height: 100vh;
            padding: 30px 20px;
        }
        .container { max-width: 780px; margin: 0 auto; }
        h1 {
            text-align: center;
            color: #fff;
            margin-bottom: 24px;
            font-size: 26px;
            text-shadow: 0 1px 3px rgba(0,0,0,0.2);
        }
        .card {
            background: #fff;
            border-radius: 14px;
            padding: 22px 26px;
            margin-bottom: 16px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .card-header {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 14px;
        }
        .card-icon {
            width: 36px; height: 36px;
            border-radius: 10px;
            display: flex; align-items: center; justify-content: center;
            font-size: 18px; flex-shrink: 0;
        }
        .icon-green { background: #e8f5e9; }
        .icon-blue { background: #e3f2fd; }
        .icon-dark { background: #f5f5f5; }
        .card h2 { font-size: 15px; color: #444; font-weight: 600; }
        .form-grid { display: flex; gap: 14px; margin-bottom: 14px; }
        .form-group { flex: 1; }
        .form-group label {
            display: block;
            font-size: 12px; color: #999;
            margin-bottom: 6px; font-weight: 500;
        }
        .form-group input {
            width: 100%;
            padding: 10px 14px;
            border: 1.5px solid #e0e0e0;
            border-radius: 10px;
            font-size: 15px;
            transition: border-color 0.15s;
        }
        .form-group input:focus { outline: none; border-color: #007aff; }
        .btn {
            display: block; width: 100%;
            padding: 13px; font-size: 15px;
            border: none; border-radius: 10px;
            cursor: pointer; transition: all 0.15s;
            font-weight: 600; letter-spacing: 0.3px;
        }
        .btn-primary {
            background: linear-gradient(135deg, #34c759, #28a745);
            color: white;
            box-shadow: 0 2px 8px rgba(52,199,89,0.3);
        }
        .btn-primary:hover:not(:disabled) {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(52,199,89,0.4);
        }
        .btn-secondary {
            background: linear-gradient(135deg, #007aff, #005ecb);
            color: white;
            box-shadow: 0 2px 8px rgba(0,122,255,0.3);
        }
        .btn-secondary:hover:not(:disabled) {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(0,122,255,0.4);
        }
        .btn:disabled {
            background: #d1d1d6 !important;
            box-shadow: none !important;
            cursor: not-allowed;
            transform: none !important;
        }
        .progress-wrap { margin-top: 14px; }
        .progress-bar {
            height: 8px; background: #e9ecef;
            border-radius: 4px; overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #34c759, #007aff);
            border-radius: 4px;
            transition: width 0.3s;
            width: 0%;
        }
        .progress-text {
            text-align: center;
            font-size: 12px; color: #999;
            margin-top: 6px;
        }
        #log {
            background: #1c1c1e;
            color: #ebebf5;
            border-radius: 10px;
            padding: 14px 16px;
            height: 420px;
            overflow-y: auto;
            font-family: "Cascadia Code", "Fira Code", Consolas, monospace;
            font-size: 12px;
            line-height: 1.85;
        }
        #log::-webkit-scrollbar { width: 5px; }
        #log::-webkit-scrollbar-track { background: transparent; }
        #log::-webkit-scrollbar-thumb { background: #555; border-radius: 3px; }
        .log-line { margin: 0; white-space: pre-wrap; word-break: break-all; }
        .log-success { color: #34c759; }
        .log-skip { color: #ff9500; }
        .log-error { color: #ff3b30; }
        .log-info { color: #5ac8fa; }
        .log-divider { color: #636366; }
    </style>
</head>
<body>
    <div class="container">
        <h1>16K 图片下载工具</h1>

        <div class="card">
            <div class="card-header">
                <div class="card-icon icon-green">&#9733;</div>
                <h2>一键下载</h2>
            </div>
            <button class="btn btn-primary" id="btnAuto" onclick="startDownload('auto')">开始一键下载</button>
            <div class="progress-wrap" id="progressAuto" style="display:none">
                <div class="progress-bar"><div class="progress-fill" id="barAuto"></div></div>
                <div class="progress-text" id="txtAuto">准备中...</div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <div class="card-icon icon-blue">&#9881;</div>
                <h2>指定页面下载</h2>
            </div>
            <div class="form-grid">
                <div class="form-group">
                    <label>Page</label>
                    <input type="number" id="page" value="1" min="1">
                </div>
                <div class="form-group">
                    <label>Size</label>
                    <input type="number" id="size" value="50" min="1" max="200">
                </div>
            </div>
            <button class="btn btn-secondary" id="btnCustom" onclick="startDownload('custom')">开始下载</button>
            <div class="progress-wrap" id="progressCustom" style="display:none">
                <div class="progress-bar"><div class="progress-fill" id="barCustom"></div></div>
                <div class="progress-text" id="txtCustom">准备中...</div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <div class="card-icon icon-dark">&#9112;</div>
                <h2>下载日志</h2>
            </div>
            <div id="log"></div>
        </div>
    </div>

    <script>
        let es, totalCount = 0, currentMode = '';

        function connectEvents() {
            if (es) es.close();
            es = new EventSource('/events');
            es.onmessage = (e) => {
                const log = document.getElementById('log');
                const line = document.createElement('div');
                line.className = 'log-line';
                line.textContent = e.data;

                if (e.data.includes('__TOTAL__:')) {
                    totalCount = parseInt(e.data.split(':')[1]);
                    return;
                }
                if (e.data.includes('__PROGRESS__:')) {
                    const parts = e.data.split(':')[1].split('/');
                    const done = parseInt(parts[0]);
                    const total = parseInt(parts[1]);
                    if (total > 0) {
                        const pct = Math.min(100, Math.round(done / total * 100));
                        const bar = currentMode === 'auto' ? document.getElementById('barAuto') : document.getElementById('barCustom');
                        const txt = currentMode === 'auto' ? document.getElementById('txtAuto') : document.getElementById('txtCustom');
                        if (bar) bar.style.width = pct + '%';
                        if (txt) txt.textContent = done + ' / ' + total;
                        if (done >= total) {
                            setTimeout(() => {
                                document.getElementById('progressAuto').style.display = 'none';
                                document.getElementById('progressCustom').style.display = 'none';
                                document.getElementById('btnAuto').disabled = false;
                                document.getElementById('btnCustom').disabled = false;
                            }, 1500);
                        }
                    }
                    return;
                }
                if (e.data.includes('跳过')) line.classList.add('log-skip');
                else if (e.data.includes('失败') || e.data.includes('错误')) line.classList.add('log-error');
                else if (e.data.includes('完成') || e.data.includes('成功')) line.classList.add('log-success');
                else if (e.data.includes('找到') || e.data.includes('访问')) line.classList.add('log-info');
                else if (e.data.includes('===')) line.classList.add('log-divider');

                log.appendChild(line);
                log.scrollTop = log.scrollHeight;
            };
            es.onerror = () => setTimeout(connectEvents, 2000);
        }
        connectEvents();

        function setButtons(disabled) {
            document.getElementById('btnAuto').disabled = disabled;
            document.getElementById('btnCustom').disabled = disabled;
        }
        async function startDownload(mode) {
            setButtons(true);
            currentMode = mode;
            totalCount = 0;
            const prog = mode === 'auto' ? 'progressAuto' : 'progressCustom';
            const bar = mode === 'auto' ? 'barAuto' : 'barCustom';
            const txt = mode === 'auto' ? 'txtAuto' : 'txtCustom';
            document.getElementById(prog).style.display = 'block';
            document.getElementById(bar).style.width = '0%';
            document.getElementById(txt).textContent = '抓取中...';
            let url = '/download?mode=' + mode;
            if (mode === 'custom') {
                url += '&page=' + encodeURIComponent(document.getElementById('page').value || '1');
                url += '&size=' + encodeURIComponent(document.getElementById('size').value || '50');
            }
            try {
                const resp = await fetch(url);
                if (!resp.ok) throw new Error(resp.status);
            } catch (e) {
                console.error(e);
                setButtons(false);
            }
        }
    </script>
</body>
</html>`

var (
	logSubs     []chan string
	logMu       sync.RWMutex
	downloading bool
	dlMu        sync.Mutex
)

func addLog(msg string) {
	msg = "[" + time.Now().Format("15:04:05") + "] " + msg
	logMu.Lock()
	for _, ch := range logSubs {
		select {
		case ch <- msg:
		default:
		}
	}
	logMu.Unlock()
	fmt.Println(msg)
}

func main() {
	http.HandleFunc("/", handlePage)
	http.HandleFunc("/download", handleDownload)
	http.HandleFunc("/events", handleEvents)

	go func() {
		log.Println("服务启动: http://localhost:8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	select {}
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(pageHTML))
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	dlMu.Lock()
	if downloading {
		dlMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"busy"}`))
		return
	}
	downloading = true
	dlMu.Unlock()

	mode := r.URL.Query().Get("mode")
	page := r.URL.Query().Get("page")
	size := r.URL.Query().Get("size")
	if mode == "auto" {
		page = "1"
		size = "50"
	}

	go func() {
		defer func() {
			dlMu.Lock()
			downloading = false
			dlMu.Unlock()
		}()

		if mode == "auto" {
			p := 1
			for {
				addLog(fmt.Sprintf("--- 翻页: page=%d ---", p))
				done := make(chan int, 1)
				go func(pageNum int) {
					done <- RunDownload(fmt.Sprintf("%d", pageNum), size, addLog, notifyTotal, func(total, current int) {
						logMu.Lock()
						for _, ch := range logSubs {
							select {
							case ch <- fmt.Sprintf("__PROGRESS__:%d/%d", current, total):
							default:
							}
						}
						logMu.Unlock()
					})
				}(p)
				count := <-done
				if count == 0 {
					addLog("已到最后一页，下载结束")
					break
				}
				p++
				time.Sleep(2 * time.Second)
			}
		} else {
			RunDownload(page, size, addLog, notifyTotal, func(total, current int) {
				logMu.Lock()
				for _, ch := range logSubs {
					select {
					case ch <- fmt.Sprintf("__PROGRESS__:%d/%d", current, total):
					default:
					}
				}
				logMu.Unlock()
			})
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func notifyTotal(n int) {
	logMu.Lock()
	for _, ch := range logSubs {
		select {
		case ch <- "__TOTAL__:" + fmt.Sprintf("%d", n):
		default:
		}
	}
	logMu.Unlock()
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan string, 200)
	logMu.Lock()
	logSubs = append(logSubs, ch)
	logMu.Unlock()

	notify := r.Context().Done()
	go func() {
		<-notify
		logMu.Lock()
		for i, s := range logSubs {
			if s == ch {
				logSubs = append(logSubs[:i], logSubs[i+1:]...)
				break
			}
		}
		logMu.Unlock()
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
