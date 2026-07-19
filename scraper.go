package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

var (
	tabCreateMu sync.Mutex
	baseURL     = "https://16k.club"
	imgDomain   = "img.16k.club"
)

type imageTask struct {
	url      string
	postID   string
	filename string
	path     string
}

func RunDownload(ctx context.Context, pageStr, sizeStr, outputBase string, onLog func(string), onTotal func(int), onProgress func(total, current int), onStats func(ok, skip, fail int)) int {
	page, size := parseParams(pageStr, sizeStr)
	onLog(fmt.Sprintf("=== 开始下载: page=%d, size=%d ===", page, size))

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	scrapeCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	postURLs := scrapeGridPage(scrapeCtx, page, size, onLog)
	if len(postURLs) == 0 {
		onLog("未找到帖子链接")
		return 0
	}

	uniquePosts := uniqueStrings(postURLs)
	onLog(fmt.Sprintf("找到 %d 个帖子（去重后）", len(uniquePosts)))

	var allImages []imageTask
	var postMu sync.Mutex
	var postWg sync.WaitGroup
	postSem := make(chan struct{}, 5)

	for _, postURL := range uniquePosts {
		postWg.Add(1)
		postSem <- struct{}{}
		go func(url string) {
			defer postWg.Done()
			defer func() { <-postSem }()

			images := scrapePostPage(allocCtx, url, onLog)
			if len(images) == 0 {
				return
			}

			postID := extractPostID(url)
			postMu.Lock()
			for _, imgURL := range images {
				allImages = append(allImages, imageTask{
					url:      imgURL,
					postID:   postID,
					filename: filepath.Base(imgURL),
				})
			}
			postMu.Unlock()
		}(postURL)
	}

	postWg.Wait()

	totalImages := len(allImages)
	if len(allImages) == 0 {
		onLog("未找到图片")
		return 0
	}

	onTotal(totalImages)
	onLog(fmt.Sprintf("共 %d 张图片", totalImages))

	os.MkdirAll(outputBase, 0755)

	const (
		maxConc  = 8
		maxRetry = 3
	)
	var dlWg sync.WaitGroup
	dlSem := make(chan struct{}, maxConc)
	var counts [3]int

	for i, task := range allImages {
		task.path = filepath.Join(outputBase, task.postID, task.filename)

		if _, err := os.Stat(task.path); err == nil {
			onLog(fmt.Sprintf("[%d/%d] 跳过: %s", i+1, totalImages, task.filename))
			counts[1]++
			onProgress(totalImages, i+1)
			onStats(counts[0], counts[1], counts[2])
			continue
		}

		dlWg.Add(1)
		dlSem <- struct{}{}
		go func(idx int, t imageTask) {
			defer dlWg.Done()
			defer func() { <-dlSem }()

			if err := os.MkdirAll(filepath.Dir(t.path), 0755); err != nil {
				onLog(fmt.Sprintf("[%d/%d] 失败: %s (目录创建失败)", idx+1, totalImages, t.filename))
				counts[2]++
				onProgress(totalImages, idx+1)
				onStats(counts[0], counts[1], counts[2])
				return
			}

			for attempt := 1; attempt <= maxRetry; attempt++ {
				if ctx.Err() != nil {
					onLog(fmt.Sprintf("[%d/%d] 中断: %s", idx+1, totalImages, t.filename))
					counts[2]++
					onProgress(totalImages, idx+1)
					onStats(counts[0], counts[1], counts[2])
					return
				}

				onLog(fmt.Sprintf("[%d/%d] 下载: %s (尝试 %d/%d)", idx+1, totalImages, t.filename, attempt, maxRetry))
				sizeBytes, err := downloadImage(allocCtx, t.url, t.path)
				if err == nil {
					onLog(fmt.Sprintf("[%d/%d] 完成: %s (%.2f MB)", idx+1, totalImages, t.filename, float64(sizeBytes)/1024/1024))
					counts[0]++
					onProgress(totalImages, idx+1)
					onStats(counts[0], counts[1], counts[2])
					return
				}

				if attempt < maxRetry {
					onLog(fmt.Sprintf("[%d/%d] 重试: %s (%v)", idx+1, totalImages, t.filename, err))
					time.Sleep(1 * time.Second)
				}
			}

			onLog(fmt.Sprintf("[%d/%d] 失败: %s (已重试 %d 次)", idx+1, totalImages, t.filename, maxRetry))
			counts[2]++
			onProgress(totalImages, idx+1)
			onStats(counts[0], counts[1], counts[2])
		}(i, task)
	}

	dlWg.Wait()
	onLog(fmt.Sprintf("=== 完成: 成功%d 跳过%d 失败%d 总计%d ===", counts[0], counts[1], counts[2], totalImages))
	return totalImages
}

func scrapeGridPage(ctx context.Context, page, size int, onLog func(string)) []string {
	pageURL := fmt.Sprintf("%s/index.php?p=%d&size=%d", baseURL, page, size)
	onLog("访问: " + pageURL)

	var html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(pageURL),
		chromedp.WaitVisible(`.grid-item`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		onLog(fmt.Sprintf("页面加载失败: %v", err))
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		onLog("HTML解析失败")
		return nil
	}

	var urls []string
	doc.Find(".grid-item").Each(func(i int, s *goquery.Selection) {
		link := s.Find("a").First()
		href, exists := link.Attr("href")
		if exists && strings.Contains(href, "/post/") {
			fullURL := resolveURL(baseURL, href)
			urls = append(urls, fullURL)
		}
	})
	return urls
}

func scrapePostPage(allocCtx context.Context, postURL string, onLog func(string)) []string {
	tabCreateMu.Lock()
	tabCtx, cancel := chromedp.NewContext(allocCtx)
	tabCreateMu.Unlock()
	defer cancel()

	ctx, cancel2 := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancel2()

	var html string
	err := chromedp.Run(ctx,
		chromedp.Navigate(postURL),
		chromedp.WaitVisible(`img`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		onLog(fmt.Sprintf("帖子加载失败: %s (%v)", extractPostID(postURL), err))
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	var urls []string
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		if strings.Contains(src, imgDomain) {
			urls = append(urls, src)
		}
	})
	return urls
}

func downloadImage(allocCtx context.Context, imgURL, path string) (int64, error) {
	tabCreateMu.Lock()
	tabCtx, cancel := chromedp.NewContext(allocCtx)
	tabCreateMu.Unlock()
	defer cancel()

	dlCtx, cancel2 := context.WithTimeout(tabCtx, 30*time.Second)
	defer cancel2()

	var data []byte
	var reqID string
	var reqIDMu sync.Mutex

	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		if ev, ok := ev.(*network.EventResponseReceived); ok {
			if strings.Contains(ev.Response.URL, imgURL) {
				reqIDMu.Lock()
				if reqID == "" {
					reqID = string(ev.RequestID)
				}
				reqIDMu.Unlock()
			}
		}
	})

	err := chromedp.Run(dlCtx,
		network.Enable(),
		chromedp.Navigate(imgURL),
		chromedp.Sleep(2*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			reqIDMu.Lock()
			rid := reqID
			reqIDMu.Unlock()

			if rid == "" {
				return fmt.Errorf("未捕获到图片响应")
			}

			s := chromedp.FromContext(ctx)
			var result struct {
				Body          string `json:"body"`
				Base64Encoded bool   `json:"base64Encoded"`
			}
			if err := s.Target.Execute(ctx, "Network.getResponseBody", map[string]any{
				"requestId": rid,
			}, &result); err != nil {
				return fmt.Errorf("getResponseBody: %w", err)
			}

			if result.Body == "" {
				return fmt.Errorf("响应体为空")
			}

			if result.Base64Encoded {
				decoded, err := decodeBase64(result.Body)
				if err != nil {
					return fmt.Errorf("解码失败: %w", err)
				}
				data = decoded
			} else {
				data = []byte(result.Body)
			}

			if len(data) < 100 {
				return fmt.Errorf("数据异常 (%d bytes)", len(data))
			}
			return nil
		}),
	)

	if err != nil {
		return 0, err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return 0, fmt.Errorf("写入失败: %w", err)
	}
	return int64(len(data)), nil
}

func parseParams(pageStr, sizeStr string) (int, int) {
	page, _ := strconv.Atoi(pageStr)
	size, _ := strconv.Atoi(sizeStr)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	return page, size
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func extractPostID(url string) string {
	parts := strings.Split(url, "/post/")
	if len(parts) < 2 {
		return "unknown"
	}
	return strings.TrimRight(parts[1], "/")
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		return "https:" + ref
	}
	if strings.HasPrefix(ref, "/") {
		return base + ref
	}
	return base + "/" + ref
}

func decodeBase64(s string) ([]byte, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(s)
}
