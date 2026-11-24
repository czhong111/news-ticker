package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRSSIncludesPublisherAndDecodesTitle(t *testing.T) {
	const document = `<rss version="2.0"><channel>
		<item><title>Markets rise &amp; investors react</title></item>
		<item><title>   </title></item>
	</channel></rss>`

	items, err := parseRSS(strings.NewReader(document), "美国 · NPR")
	if err != nil {
		t.Fatalf("parseRSS returned an error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Source != "美国 · NPR" || items[0].Title != "Markets rise & investors react" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

func TestParseRSSRejectsMalformedXML(t *testing.T) {
	if _, err := parseRSS(strings.NewReader(`<rss><channel>`), "test"); err == nil {
		t.Fatal("parseRSS accepted malformed XML")
	}
}

func TestRefreshNewsFailureDoesNotUseMockData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	originalFeeds := feeds
	dataMu.Lock()
	originalItems := newsItems
	originalText := tickerText
	originalVersion := tickerVersion
	newsItems = nil
	tickerText = "正在连接境外新闻源…"
	tickerVersion = 0
	dataMu.Unlock()
	feeds = []feed{{source: "test", url: server.URL}}
	defer func() {
		feeds = originalFeeds
		dataMu.Lock()
		newsItems = originalItems
		tickerText = originalText
		tickerVersion = originalVersion
		dataMu.Unlock()
	}()

	refreshNewsWithTranslator(server.Client(), func(title string) (string, error) {
		return title, nil
	})

	dataMu.RLock()
	defer dataMu.RUnlock()
	if !strings.Contains(tickerText, "无法连接境外新闻源") {
		t.Fatalf("expected a connection status, got %q", tickerText)
	}
}

func TestRefreshNewsDisplaysSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><item><title>Test headline</title></item></channel></rss>`))
	}))
	defer server.Close()

	originalFeeds := feeds
	dataMu.Lock()
	originalItems := newsItems
	originalText := tickerText
	originalVersion := tickerVersion
	newsItems = nil
	tickerText = ""
	tickerVersion = 0
	dataMu.Unlock()
	feeds = []feed{{source: "新加坡 · CNA", url: server.URL}}
	defer func() {
		feeds = originalFeeds
		dataMu.Lock()
		newsItems = originalItems
		tickerText = originalText
		tickerVersion = originalVersion
		dataMu.Unlock()
	}()

	refreshNewsWithTranslator(server.Client(), func(string) (string, error) {
		return "测试标题译文", nil
	})

	dataMu.RLock()
	defer dataMu.RUnlock()
	if !strings.Contains(tickerText, "[新加坡 · CNA] 测试标题译文") {
		t.Fatalf("publisher was not displayed: %q", tickerText)
	}
}

func TestRequestTranslationFrom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "Central bank holds rates" {
			t.Errorf("query title = %q", got)
		}
		if got := r.URL.Query().Get("langpair"); got != "en|zh-CN" {
			t.Errorf("language pair = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responseStatus": 200,
			"quotaFinished":  false,
			"responseData": map[string]string{
				"translatedText": "央行维持利率不变",
			},
		})
	}))
	defer server.Close()

	translated, err := requestTranslationFrom(server.Client(), server.URL, "Central bank holds rates")
	if err != nil {
		t.Fatalf("requestTranslationFrom returned an error: %v", err)
	}
	if translated != "央行维持利率不变" {
		t.Fatalf("translation = %q", translated)
	}
}

func TestTranslateNewsItemsMarksOriginalOnFailure(t *testing.T) {
	items := []newsItem{{Source: "美国 · NPR", Title: "Original headline"}}
	failures := translateNewsItems(items, func(string) (string, error) {
		return "", http.ErrHandlerTimeout
	})
	if failures != 1 || items[0].Source != "美国 · NPR · 原文" {
		t.Fatalf("unexpected fallback: failures=%d item=%#v", failures, items[0])
	}
}
