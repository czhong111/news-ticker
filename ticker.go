package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	defaultWindowHeight = 46
	controlPanelWidth   = 148
	timerID             = 1
	timerPeriod         = 33

	wmDestroy     = 0x0002
	wmSize        = 0x0005
	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmNcHitTest   = 0x0084
	wmTimer       = 0x0113
	wmLButtonDown = 0x0201

	wsPopup      = 0x80000000
	wsThickFrame = 0x00040000
	wsExTopmost  = 0x00000008
	wsExToolwind = 0x00000080

	spiGetWorkArea = 0x0030
	transparent    = 1

	htClient      = 1
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procShowWindow             = user32.NewProc("ShowWindow")
	procUpdateWindow           = user32.NewProc("UpdateWindow")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procSetTimer               = user32.NewProc("SetTimer")
	procKillTimer              = user32.NewProc("KillTimer")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procSystemParametersInfoW  = user32.NewProc("SystemParametersInfoW")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procBeginPaint             = user32.NewProc("BeginPaint")
	procEndPaint               = user32.NewProc("EndPaint")
	procInvalidateRect         = user32.NewProc("InvalidateRect")
	procMessageBoxW            = user32.NewProc("MessageBoxW")
	procGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procFillRect               = user32.NewProc("FillRect")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procTextOutW               = gdi32.NewProc("TextOutW")
	procGetTextExtent          = gdi32.NewProc("GetTextExtentPoint32W")
	procSaveDC                 = gdi32.NewProc("SaveDC")
	procRestoreDC              = gdi32.NewProc("RestoreDC")
	procIntersectClip          = gdi32.NewProc("IntersectClipRect")
)

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type message struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	SmallIcon  uintptr
}

type appConfig struct {
	FontSizePt          int `json:"fontSizePt"`
	RefreshIntervalSecs int `json:"refreshIntervalSeconds"`
	ScrollSpeed         int `json:"scrollPixelsPerSecond"`
}

type rssItem struct {
	Title string `xml:"title"`
}

type rssDocument struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type newsItem struct {
	Source string
	Title  string
}

type feed struct {
	source string
	url    string
}

type translationResponse struct {
	ResponseData struct {
		TranslatedText string `json:"translatedText"`
	} `json:"responseData"`
	ResponseStatus  int    `json:"responseStatus"`
	QuotaFinished   bool   `json:"quotaFinished"`
	ResponseDetails string `json:"responseDetails"`
}

type cachedTranslation struct {
	text       string
	retryAfter time.Time
}

var feeds = []feed{
	{source: "美国 · NPR", url: "https://feeds.npr.org/1001/rss.xml"},
	{source: "欧洲 · The Guardian", url: "https://www.theguardian.com/world/rss"},
	{source: "欧洲 · France 24", url: "https://www.france24.com/en/rss"},
	{source: "香港 · RTHK", url: "https://rthk.hk/rthk/news/rss/e_expressnews_clocal.xml"},
	{source: "香港 · SCMP", url: "https://www.scmp.com/rss/91/feed"},
	{source: "新加坡 · CNA", url: "https://www.channelnewsasia.com/rssfeeds/8395886"},
	{source: "日本 · NHK WORLD", url: "https://www3.nhk.or.jp/nhkworld/en/news/list/rss.xml"},
	{source: "日本 · Japan Times", url: "https://www.japantimes.co.jp/feed/"},
}

var (
	dataMu            sync.RWMutex
	translationCache  sync.Map
	translationClient = &http.Client{Timeout: 8 * time.Second}
	newsItems         []newsItem
	tickerText        = "正在连接境外新闻源…"
	tickerVersion     uint64

	config                      = appConfig{FontSizePt: 12, RefreshIntervalSecs: 300, ScrollSpeed: 60}
	fontHeight            int32 = 16
	scrollOffset          float64
	tickerSpan            int32
	tickerMeasuredVersion uint64
	paused                bool
	windowHandle          uintptr
	tickerFont            uintptr
	controlFont           uintptr
	backgroundBrush       uintptr
	controlBrush          uintptr
)

func main() {
	if err := run(); err != nil {
		showError(err)
		os.Exit(1)
	}
}

func run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := loadConfig(); err != nil {
		return err
	}

	instance, _, _ := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return errors.New("无法获取程序实例")
	}

	className, _ := syscall.UTF16PtrFromString("NorthstarCompactTicker")
	title, _ := syscall.UTF16PtrFromString("国际新闻滚动条")
	cursor, _, _ := procLoadCursorW.Call(0, 32512)
	class := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(windowProc),
		Instance:  instance,
		Cursor:    cursor,
		ClassName: className,
	}
	if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return fmt.Errorf("注册窗口失败: %v", callErr)
	}

	tickerFont = createFont(-fontHeight, 400)
	controlFont = tickerFont
	backgroundBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(10, 15, 23)))
	controlBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(18, 27, 38)))
	if tickerFont == 0 || backgroundBrush == 0 || controlBrush == 0 {
		cleanupGDI()
		return errors.New("初始化窗口绘图资源失败")
	}

	area, err := getWorkArea()
	if err != nil {
		cleanupGDI()
		return err
	}
	windowHandle, _, _ = procCreateWindowExW.Call(
		wsExTopmost|wsExToolwind,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup|wsThickFrame|0x10000000,
		uintptr(area.Left),
		uintptr(area.Bottom-defaultWindowHeight),
		uintptr(area.Right-area.Left),
		defaultWindowHeight,
		0, 0, instance, 0,
	)
	if windowHandle == 0 {
		cleanupGDI()
		return errors.New("创建新闻条窗口失败")
	}

	if timer, _, callErr := procSetTimer.Call(windowHandle, timerID, timerPeriod, 0); timer == 0 {
		procDestroyWindow.Call(windowHandle)
		cleanupGDI()
		return fmt.Errorf("启动滚动动画失败: %v", callErr)
	}
	procShowWindow.Call(windowHandle, 5)
	procUpdateWindow.Call(windowHandle)
	go refreshNewsLoop()

	var msg message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			procKillTimer.Call(windowHandle, timerID)
			cleanupGDI()
			return fmt.Errorf("读取窗口消息失败: %v", callErr)
		}
		if result == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	cleanupGDI()
	return nil
}

func loadConfig() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("读取程序路径失败: %w", err)
	}
	file, err := os.Open(filepath.Join(filepath.Dir(executable), "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer file.Close()

	var loaded appConfig
	if err := json.NewDecoder(file).Decode(&loaded); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	if loaded.FontSizePt < 8 || loaded.FontSizePt > 32 {
		return errors.New("config.json 的 fontSizePt 必须在 8 到 32 之间")
	}
	if loaded.RefreshIntervalSecs < 30 || loaded.RefreshIntervalSecs > 86400 {
		return errors.New("config.json 的 refreshIntervalSeconds 必须在 30 到 86400 之间")
	}
	if loaded.ScrollSpeed < 10 || loaded.ScrollSpeed > 500 {
		return errors.New("config.json 的 scrollPixelsPerSecond 必须在 10 到 500 之间")
	}
	config = loaded
	fontHeight = int32(config.FontSizePt * 4 / 3)
	return nil
}

func createFont(height, weight int32) uintptr {
	name, _ := syscall.UTF16PtrFromString("Microsoft YaHei UI")
	font, _, _ := procCreateFontW.Call(
		uintptr(height), 0, 0, 0, uintptr(weight),
		0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(name)),
	)
	return font
}

func cleanupGDI() {
	for _, handle := range []uintptr{tickerFont, backgroundBrush, controlBrush} {
		if handle != 0 {
			procDeleteObject.Call(handle)
		}
	}
}

func getWorkArea() (rect, error) {
	var area rect
	result, _, callErr := procSystemParametersInfoW.Call(
		spiGetWorkArea,
		0,
		uintptr(unsafe.Pointer(&area)),
		0,
	)
	if result == 0 {
		return rect{}, fmt.Errorf("读取屏幕工作区失败: %v", callErr)
	}
	return area, nil
}

func dockWindow(hwnd uintptr) error {
	var bounds rect
	if result, _, callErr := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds))); result == 0 {
		return fmt.Errorf("读取新闻条位置失败: %v", callErr)
	}
	area, err := getWorkArea()
	if err != nil {
		return err
	}
	width := bounds.Right - bounds.Left
	height := bounds.Bottom - bounds.Top
	if width > area.Right-area.Left {
		width = area.Right - area.Left
	}
	result, _, callErr := procSetWindowPos.Call(
		hwnd,
		^uintptr(0),
		uintptr(area.Left),
		uintptr(area.Bottom-height),
		uintptr(width),
		uintptr(height),
		0x0040,
	)
	if result == 0 {
		return fmt.Errorf("贴靠任务栏失败: %v", callErr)
	}
	return nil
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmNcHitTest:
		return hitTestWindow(hwnd, lParam)
	case wmSize:
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmPaint:
		paintWindow(hwnd)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmTimer:
		if wParam == timerID && !paused {
			scrollOffset += float64(config.ScrollSpeed) * float64(timerPeriod) / 1000
			var bounds rect
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
			tickerArea := rect{
				Left:   0,
				Top:    0,
				Right:  bounds.Right - controlPanelWidth,
				Bottom: bounds.Bottom,
			}
			procInvalidateRect.Call(hwnd, uintptr(unsafe.Pointer(&tickerArea)), 0)
		}
		return 0
	case wmLButtonDown:
		var bounds rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
		x := int32(int16(lParam & 0xffff))
		switch {
		case x >= bounds.Right-controlPanelWidth && x < bounds.Right-96:
			paused = !paused
			controls := rect{Left: bounds.Right - controlPanelWidth, Top: 0, Right: bounds.Right, Bottom: bounds.Bottom}
			procInvalidateRect.Call(hwnd, uintptr(unsafe.Pointer(&controls)), 0)
			return 0
		case x >= bounds.Right-96 && x < bounds.Right-44:
			if err := dockWindow(hwnd); err != nil {
				showError(err)
			}
			return 0
		case x >= bounds.Right-44:
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmDestroy:
		procKillTimer.Call(hwnd, timerID)
		procPostQuitMessage.Call(0)
		return 0
	}

	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func hitTestWindow(hwnd, lParam uintptr) uintptr {
	var bounds rect
	if result, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds))); result == 0 {
		return htClient
	}
	x := int32(int16(lParam & 0xffff))
	y := int32(int16((lParam >> 16) & 0xffff))
	const border = 8

	left := x < bounds.Left+border
	right := x >= bounds.Right-border
	top := y < bounds.Top+border
	bottom := y >= bounds.Bottom-border
	switch {
	case left && top:
		return htTopLeft
	case right && top:
		return htTopRight
	case left && bottom:
		return htBottomLeft
	case right && bottom:
		return htBottomRight
	case left:
		return htLeft
	case right:
		return htRight
	case top:
		return htTop
	case bottom:
		return htBottom
	case x >= bounds.Right-controlPanelWidth:
		return htClient
	default:
		return htCaption
	}
}

func paintWindow(hwnd uintptr) {
	var ps paintStruct
	targetDC, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var bounds rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
	memoryDC, _, _ := procCreateCompatibleDC.Call(targetDC)
	if memoryDC == 0 {
		drawWindow(targetDC, bounds)
		return
	}
	bitmap, _, _ := procCreateCompatibleBitmap.Call(targetDC, uintptr(bounds.Right), uintptr(bounds.Bottom))
	if bitmap == 0 {
		procDeleteDC.Call(memoryDC)
		drawWindow(targetDC, bounds)
		return
	}
	oldBitmap, _, _ := procSelectObject.Call(memoryDC, bitmap)
	drawWindow(memoryDC, bounds)
	if result, _, _ := procBitBlt.Call(
		targetDC, 0, 0, uintptr(bounds.Right), uintptr(bounds.Bottom),
		memoryDC, 0, 0, 0x00CC0020,
	); result == 0 {
		drawWindow(targetDC, bounds)
	}
	procSelectObject.Call(memoryDC, oldBitmap)
	procDeleteObject.Call(bitmap)
	procDeleteDC.Call(memoryDC)
}

func drawWindow(hdc uintptr, bounds rect) {
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&bounds)), backgroundBrush)

	dataMu.RLock()
	text := tickerText
	version := tickerVersion
	dataMu.RUnlock()

	width := bounds.Right
	clipRight := width - controlPanelWidth - 8
	if clipRight < 16 {
		clipRight = 16
	}
	savedDC, _, _ := procSaveDC.Call(hdc)
	procIntersectClip.Call(hdc, 8, 0, uintptr(clipRight), uintptr(bounds.Bottom))

	baseText := text + "      "
	repeatedText := baseText + text
	if version != tickerMeasuredVersion {
		tickerSpan = measureText(hdc, baseText, tickerFont)
		tickerMeasuredVersion = version
		scrollOffset = 0
	}
	if tickerSpan > 0 {
		if scrollOffset >= float64(tickerSpan) {
			scrollOffset = 0
		}
		y := int((bounds.Bottom - fontHeight) / 2)
		drawText(hdc, 8-int(scrollOffset), y, repeatedText, rgb(235, 239, 244), tickerFont)
	}
	procRestoreDC.Call(hdc, savedDC)

	controls := rect{Left: width - controlPanelWidth, Top: 0, Right: width, Bottom: bounds.Bottom}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&controls)), controlBrush)
	pauseLabel := "暂停"
	if paused {
		pauseLabel = "继续"
	}
	y := int((bounds.Bottom - fontHeight) / 2)
	drawText(hdc, int(width)-140, y, pauseLabel, rgb(92, 207, 220), controlFont)
	drawText(hdc, int(width)-88, y, "贴底", rgb(143, 160, 177), controlFont)
	drawText(hdc, int(width)-34, y, "×", rgb(190, 202, 214), controlFont)
}

func drawText(hdc uintptr, x, y int, text string, color uint32, font uintptr) {
	if text == "" {
		return
	}
	encoded := utf16.Encode([]rune(text))
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	procSetTextColor.Call(hdc, uintptr(color))
	procSetBkMode.Call(hdc, transparent)
	procTextOutW.Call(
		hdc,
		uintptr(int32(x)),
		uintptr(int32(y)),
		uintptr(unsafe.Pointer(&encoded[0])),
		uintptr(len(encoded)),
	)
	procSelectObject.Call(hdc, oldFont)
}

func measureText(hdc uintptr, text string, font uintptr) int32 {
	encoded := utf16.Encode([]rune(text))
	if len(encoded) == 0 {
		return 0
	}
	var size point
	oldFont, _, _ := procSelectObject.Call(hdc, font)
	procGetTextExtent.Call(
		hdc,
		uintptr(unsafe.Pointer(&encoded[0])),
		uintptr(len(encoded)),
		uintptr(unsafe.Pointer(&size)),
	)
	procSelectObject.Call(hdc, oldFont)
	return size.X
}

func rgb(r, g, b uint32) uint32 {
	return r | g<<8 | b<<16
}

func refreshNewsLoop() {
	client := &http.Client{Timeout: 12 * time.Second}
	for {
		refreshNews(client)
		time.Sleep(time.Duration(config.RefreshIntervalSecs) * time.Second)
	}
}

func refreshNews(client *http.Client) {
	refreshNewsWithTranslator(client, translateHeadline)
}

func refreshNewsWithTranslator(client *http.Client, translate func(string) (string, error)) {
	type result struct {
		index int
		items []newsItem
		err   error
	}
	results := make(chan result, len(feeds))
	for index, source := range feeds {
		index := index
		source := source
		go func() {
			items, err := fetchFeed(client, source)
			results <- result{index: index, items: items, err: err}
		}()
	}

	feedResults := make([]result, len(feeds))
	successes := 0
	for range feeds {
		response := <-results
		feedResults[response.index] = response
		if response.err == nil {
			successes++
		}
	}

	combined := make([]newsItem, 0, 24)
	seen := make(map[string]bool)
	for itemIndex := 0; itemIndex < 3; itemIndex++ {
		for _, response := range feedResults {
			if response.err != nil || itemIndex >= len(response.items) || len(combined) >= cap(combined) {
				continue
			}
			item := response.items[itemIndex]
			key := strings.ToLower(strings.TrimSpace(item.Title))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			combined = append(combined, item)
		}
	}

	translationErrors := translateNewsItems(combined, translate)
	dataMu.Lock()
	if len(combined) > 0 {
		newsItems = combined
		parts := make([]string, 0, len(newsItems))
		for _, item := range newsItems {
			source := item.Source
			if item.Title == "" {
				continue
			}
			parts = append(parts, "["+source+"] "+item.Title)
		}
		if translationErrors > 0 {
			parts = append(parts, fmt.Sprintf("（%d 条标题暂显示原文）", translationErrors))
		}
		tickerText = strings.Join(parts, "      •      ")
		tickerVersion++
	} else if len(newsItems) > 0 {
		tickerText = "境外新闻源暂时连接失败，继续显示上次更新的新闻。"
		tickerVersion++
	} else if successes == 0 {
		tickerText = "无法连接境外新闻源，请检查网络；程序会自动重试。"
		tickerVersion++
	} else {
		tickerText = "境外新闻源暂时没有可显示的新闻，程序会自动重试。"
		tickerVersion++
	}
	dataMu.Unlock()

	if windowHandle != 0 {
		procInvalidateRect.Call(windowHandle, 0, 0)
	}
}

func translateNewsItems(items []newsItem, translate func(string) (string, error)) int {
	const maxConcurrentTranslations = 6
	semaphore := make(chan struct{}, maxConcurrentTranslations)
	var workers sync.WaitGroup
	var failureMu sync.Mutex
	failures := 0

	for index := range items {
		semaphore <- struct{}{}
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			defer func() { <-semaphore }()

			original := items[index].Title
			translated, err := translate(original)
			if err != nil || strings.TrimSpace(translated) == "" {
				items[index].Source += " · 原文"
				failureMu.Lock()
				failures++
				failureMu.Unlock()
				return
			}
			items[index].Title = strings.TrimSpace(translated)
		}(index)
	}
	workers.Wait()
	return failures
}

func translateHeadline(title string) (string, error) {
	key := strings.TrimSpace(title)
	if cached, ok := translationCache.Load(key); ok {
		entry := cached.(cachedTranslation)
		if entry.text != "" {
			return entry.text, nil
		}
		if time.Now().Before(entry.retryAfter) {
			return "", errors.New("translation temporarily unavailable")
		}
	}

	translated, err := requestTranslation(key)
	if err != nil {
		translationCache.Store(key, cachedTranslation{retryAfter: time.Now().Add(10 * time.Minute)})
		return "", err
	}
	translationCache.Store(key, cachedTranslation{text: translated})
	return translated, nil
}

func requestTranslation(title string) (string, error) {
	return requestTranslationFrom(translationClient, "https://api.mymemory.translated.net/get", title)
}

func requestTranslationFrom(client *http.Client, endpointURL, title string) (string, error) {
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("q", title)
	query.Set("langpair", "en|zh-CN")
	query.Set("mt", "1")
	endpoint.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "NorthstarCompactTicker/1.0")

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("translation service returned HTTP %d", response.StatusCode)
	}

	var result translationResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if result.ResponseStatus != http.StatusOK || result.QuotaFinished {
		return "", fmt.Errorf("translation service unavailable: %s", result.ResponseDetails)
	}
	translated := strings.TrimSpace(html.UnescapeString(result.ResponseData.TranslatedText))
	if translated == "" {
		return "", errors.New("translation service returned an empty title")
	}
	return translated, nil
}

func fetchFeed(client *http.Client, source feed) ([]newsItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "NorthstarCompactTicker/1.0")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("RSS 请求返回 HTTP %d", response.StatusCode)
	}
	items, err := parseRSS(io.LimitReader(response.Body, 2<<20), source.source)
	if err != nil {
		return nil, err
	}
	if len(items) > 3 {
		items = items[:3]
	}
	return items, nil
}

func parseRSS(reader io.Reader, source string) ([]newsItem, error) {
	var document rssDocument
	if err := xml.NewDecoder(reader).Decode(&document); err != nil {
		return nil, err
	}
	items := make([]newsItem, 0, len(document.Channel.Items))
	for _, entry := range document.Channel.Items {
		title := strings.Join(strings.Fields(html.UnescapeString(entry.Title)), " ")
		if title != "" {
			items = append(items, newsItem{Source: source, Title: title})
		}
	}
	return items, nil
}

func showError(err error) {
	message, _ := syscall.UTF16PtrFromString(err.Error())
	title, _ := syscall.UTF16PtrFromString("News Ticker")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
