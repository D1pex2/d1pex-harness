package wait

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// TB — то, чего ожидания и обёртка браузера требуют от прогона. *testing.T
// подходит как есть; интерактивный CLI подставляет свою реализацию, где
// Fatalf завершает процесс, а ArtifactDir указывает в каталог сессии.
type TB interface {
	Helper()
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
	Failed() bool
	ArtifactDir() string
}

// Interval — период опроса по умолчанию.
//
// Всё, что опрашивается ожиданиями, живёт в контейнерах стенда: Mongo,
// Temporal, моки каналов. Они догоняют состояние за десятки миллисекунд, и
// такт в полсекунды означал только простой: событие, случившееся на 120ms,
// замечалось на 500ms. Боевой Трекер в циклах не опрашивается — он нужен
// лишь в Setup, где вызовы одиночные, поэтому его лимиты такт не задевает.
const Interval = 100 * time.Millisecond

// ReportEvery — как часто затянувшееся ожидание отчитывается в лог теста.
const ReportEvery = 15 * time.Second

// OnSlow зовётся вместе с каждым отчётом затянувшегося ожидания. Пакет browser
// вешает сюда дамп вкладки: ждать полного дедлайна ради снимка состояния глупо,
// а прерванный руками прогон не доходит ни до Cleanup, ни до падения — и тогда
// не остаётся вообще ничего. nil, пока никто не подписался.
var OnSlow func(what string)

// Until опрашивает fn каждые Interval, пока та не вернёт ok или не истечёт ctx.
// Дедлайн задаётся вызывающим через context.WithTimeout.
//
//	// Бронь появилась в Mongo.
//	b := wait.Until(ctx, t, func() (domain.Booking, bool) {
//		return db.BookingByBeds24ID(ctx, id)
//	})
//
// Описание берётся из комментария перед вызовом.
func Until[T any](ctx context.Context, t TB, fn func() (T, bool)) T {
	t.Helper()
	return UntilEvery(ctx, t, Interval, fn, describe(2, nil))
}

// UntilEvery — Until со своим тактом.
//
// Такт принадлежит тому, кого опрашивают: DOM перерисовывается за миллисекунды,
// воркфлоу догоняет секундами. Одно число на всех неизбежно ставится по самому
// медленному участнику, и по нему потом ждут все остальные.
func UntilEvery[T any](ctx context.Context, t TB, interval time.Duration, fn func() (T, bool), description ...string) T {
	t.Helper()
	what := describe(2, description)

	started := time.Now()
	nextReport := started.Add(ReportEvery)

	var last T
	for {
		v, ok := fn()
		if ok {
			return v
		}
		last = v

		// Долгое ожидание обязано говорить, чего оно ждёт. Иначе минуты до
		// дедлайна выглядят как зависший прогон, и понять, ждём мы медленное
		// или сломанное, можно только по факту падения.
		if now := time.Now(); now.After(nextReport) {
			t.Logf("жду: %s — %v, последнее значение: %+v",
				what, now.Sub(started).Round(time.Second), last)
			nextReport = now.Add(ReportEvery)
			if OnSlow != nil {
				OnSlow(what)
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("не дождались: %s (%v); последнее значение: %+v", what, ctx.Err(), last)
			return last
		case <-time.After(interval):
		}
	}
}

// Consistently проверяет, что условие остаётся истинным весь duration.
func Consistently(ctx context.Context, t TB, duration time.Duration, fn func() bool, description ...string) {
	t.Helper()
	observe(ctx, t, duration, fn, true, describe(2, description))
}

// Never проверяет, что условие не станет истинным в течение duration.
func Never(ctx context.Context, t TB, duration time.Duration, fn func() bool, description ...string) {
	t.Helper()
	observe(ctx, t, duration, fn, false, describe(2, description))
}

func observe(ctx context.Context, t TB, duration time.Duration, fn func() bool, want bool, what string) {
	t.Helper()
	if duration <= 0 {
		t.Fatalf("длительность проверки %q должна быть положительной", what)
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()

	for {
		if got := fn(); got != want {
			if want {
				t.Fatalf("условие перестало выполняться: %s", what)
			}
			t.Fatalf("произошло запрещённое событие: %s", what)
		}

		select {
		case <-ctx.Done():
			t.Fatalf("проверка прервана: %s (%v)", what, ctx.Err())
		case <-timer.C:
			return
		case <-ticker.C:
		}
	}
}

func describe(skip int, description []string) string {
	if len(description) > 0 && description[0] != "" {
		return description[0]
	}

	_, path, line, ok := runtime.Caller(skip)
	if !ok {
		return "условие"
	}

	data, err := os.ReadFile(path)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for i := line - 2; i >= 0; i-- {
			text := strings.TrimSpace(lines[i])
			if strings.HasPrefix(text, "//") {
				return strings.TrimSpace(strings.TrimPrefix(text, "//"))
			}
			if text != "" {
				break
			}
		}
	}

	return fmt.Sprintf("%s:%d", filepath.Base(path), line)
}
