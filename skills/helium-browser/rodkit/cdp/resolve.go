package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// DefaultAddr — порт отладки, на котором поднимают живой браузер.
const DefaultAddr = "127.0.0.1:9222"

// Resolve находит ws-адрес CDP браузера, который слушает порт отладки.
//
// Принимает порт, host:port, http-адрес или прежний ws-адрес — от последнего
// берётся только host:port: id браузера в пути меняется с каждым запуском, и
// после перезапуска старый адрес ведёт в никуда, хотя порт тот же. Пустая
// строка — DefaultAddr.
//
// launcher.ResolveURL из rod делает то же, но без таймаута: порт, занятый
// чем-то молчащим, подвесил бы команду навсегда.
func Resolve(addr string) (string, error) {
	host, err := HostPort(addr)
	if err != nil {
		return "", err
	}
	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://" + host + "/json/version")
	if err != nil {
		return "", fmt.Errorf("порт отладки %s не отвечает: %w", host, err)
	}
	defer res.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil || v.WS == "" {
		return "", fmt.Errorf("на %s отвечает не браузер с CDP", host)
	}
	ws, err := url.Parse(v.WS)
	if err != nil {
		return "", fmt.Errorf("браузер отдал странный адрес %q: %w", v.WS, err)
	}
	// Браузер называет себя localhost или 127.0.0.1 как ему вздумается; идём
	// туда же, куда спрашивали.
	ws.Host = host
	return ws.String(), nil
}

// HostPort вытаскивает host:port из того, что дали: «9222», «:9222»,
// «host:9222» или URL любой схемы.
func HostPort(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DefaultAddr, nil
	}
	if port := strings.TrimPrefix(addr, ":"); isPort(port) {
		return "127.0.0.1:" + port, nil
	}
	if !strings.Contains(addr, "://") {
		return addr, nil
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("не понимаю адрес %q: нужен порт, host:port или URL", addr)
	}
	return u.Host, nil
}

func isPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536
}

// BrowserPID — pid процесса браузера. Домен SystemInfo браузерный, страница о
// запросе не узнаёт. 0, если браузер не сказал.
func (s *Session) BrowserPID() int {
	res, err := proto.SystemInfoGetProcessInfo{}.Call(s.browser)
	if err != nil {
		return 0
	}
	for _, p := range res.ProcessInfo {
		if p.Type == "browser" {
			return p.ID
		}
	}
	return 0
}

// Product — сборка браузера, «Chrome/153.0.…».
func (s *Session) Product() string {
	res, err := proto.BrowserGetVersion{}.Call(s.browser)
	if err != nil {
		return ""
	}
	return res.Product
}
