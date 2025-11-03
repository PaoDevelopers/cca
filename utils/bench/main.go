// Test the performance of our server
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	baseURL     = "https://paospace.ykpaoschool.cn:8192"
	concurrency = 100
	endpoints   = []string{
		"/student/",
		"/student/api/user_info",
		"/student/api/courses",
		"/student/api/my_selections",
		"/student/api/grades",
		"/student/api/periods",
	}
	authTimeout    = 8 * time.Second
	requestTimeout = 10 * time.Second
)

type resultSummary struct {
	totalRequests int64
	failures      int64
	totalTimeNs   int64
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("cannot find home directory:", err)
		return
	}
	filePath := filepath.Join(home, "students.txt")
	ids, err := readStudentIDs(filePath)
	if err != nil {
		fmt.Println("error reading student IDs:", err)
		return
	}

	numStudents := len(ids)
	fmt.Printf("Simulating %d students, concurrency=%d, baseURL=%s\n", numStudents, concurrency, baseURL)

	start := time.Now()
	students := make(chan string, numStudents)
	for _, id := range ids {
		students <- id
	}
	close(students)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	var summary resultSummary

	for id := range students {
		wg.Add(1)
		sem <- struct{}{}
		go func(studentID string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := handleStudent(studentID, &summary); err != nil {
				fmt.Printf("[student %s] error: %v\n", studentID, err)
			}
		}(id)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalReq := atomic.LoadInt64(&summary.totalRequests)
	fail := atomic.LoadInt64(&summary.failures)
	avgReqMs := float64(0)
	if totalReq-fail > 0 {
		avgReqMs = float64(atomic.LoadInt64(&summary.totalTimeNs)) / float64(totalReq-fail) / 1e6
	}

	fmt.Println("==== summary ====")
	fmt.Printf("students attempted: %d\n", numStudents)
	fmt.Printf("total requests attempted: %d\n", totalReq)
	fmt.Printf("failed requests: %d\n", fail)
	fmt.Printf("avg successful request latency: %.2f ms\n", avgReqMs)
	fmt.Printf("wall-clock elapsed: %s\n", elapsed)
}

func readStudentIDs(path string) ([]string, error) {
	f, err := os.Open(path) //#nosec:G304
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var ids []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ids = append(ids, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func handleStudent(studentID string, summary *resultSummary) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: requestTimeout,
	}

	authURL := strings.TrimRight(baseURL, "/") + "/auth"
	form := url.Values{}
	form.Set("bypass", studentID)

	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		atomic.AddInt64(&summary.failures, 1)
		atomic.AddInt64(&summary.totalRequests, 1)
		return fmt.Errorf("auth request failed: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	atomic.AddInt64(&summary.totalRequests, 1)
	atomic.AddInt64(&summary.totalTimeNs, time.Since(start).Nanoseconds())
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		atomic.AddInt64(&summary.failures, 1)
		return fmt.Errorf("auth returned status %d", resp.StatusCode)
	}

	u, _ := url.Parse(baseURL)
	cookies := client.Jar.Cookies(u)
	if len(cookies) == 0 {
		fmt.Printf("[student %s] warning: no cookies after auth\n", studentID)
	}

	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()
			epURL := strings.TrimRight(baseURL, "/") + endpoint
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, epURL, nil)
			if err != nil {
				atomic.AddInt64(&summary.failures, 1)
				atomic.AddInt64(&summary.totalRequests, 1)
				fmt.Printf("[student %s] create request %s: %v\n", studentID, endpoint, err)
				return
			}
			req.Header.Set("Accept", "application/json, text/html;q=0.8, */*;q=0.1")

			start := time.Now()
			resp, err := client.Do(req)
			lat := time.Since(start)
			atomic.AddInt64(&summary.totalRequests, 1)
			atomic.AddInt64(&summary.totalTimeNs, lat.Nanoseconds())
			if err != nil {
				atomic.AddInt64(&summary.failures, 1)
				fmt.Printf("[student %s] GET %s error: %v\n", studentID, endpoint, err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				atomic.AddInt64(&summary.failures, 1)
				fmt.Printf("[student %s] GET %s -> %d\n", studentID, endpoint, resp.StatusCode)
				return
			}
		}(ep)
	}

	wg.Wait()
	return nil
}
