//go:build windows

package win

import (
	"time"
	"unsafe"
)

var qpcFreq = func() int64 {
	var f int64
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&f)))
	return f
}()

// QPC returns the current QueryPerformanceCounter value (ticks). This is the same scale
// as DXGI_OUTDUPL_FRAME_INFO.LastPresentTime and DXGI_FRAME_STATISTICS.SyncQPCTime.
func QPC() int64 {
	var c int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&c)))
	return c
}

// QPF is the QPC frequency (ticks per second).
func QPF() int64 { return qpcFreq }

func TicksToMs(t int64) float64 { return float64(t) * 1000 / float64(qpcFreq) }

func MsToTicks(ms float64) int64 { return int64(ms * float64(qpcFreq) / 1000) }

func DurToTicks(d time.Duration) int64 { return int64(d) * qpcFreq / int64(time.Second) }

// Sleeper sleeps precisely until a QPC moment: a high-resolution waitable timer, topped
// off with a spin wait.
type Sleeper struct {
	h    uintptr
	spin int64 // how many ticks at the end to spin-wait for precise sleeps
}

// NewSleeper creates the timer (CREATE_WAITABLE_TIMER_HIGH_RESOLUTION, Windows 10 1803+;
// falls back to a regular one otherwise).
func NewSleeper() *Sleeper {
	h, _, _ := procCreateWaitableTimerExW.Call(0, 0, createWaitableTimerHighResolution, timerAllAccess)
	if h == 0 {
		h, _, _ = procCreateWaitableTimerExW.Call(0, 0, 0, timerAllAccess)
	}
	return &Sleeper{h: h, spin: MsToTicks(0.25)}
}

// SleepUntil sleeps until moment deadline (QPC ticks).
// precise=true spin-waits the last ~0.25 ms (for a Present moment);
// precise=false uses only the timer (for polling, to save CPU).
func (s *Sleeper) SleepUntil(deadline int64, precise bool) {
	for {
		remain := deadline - QPC()
		if remain <= 0 {
			return
		}
		if precise && remain <= s.spin {
			for QPC() < deadline {
			}
			return
		}
		wait := remain
		if precise {
			wait -= s.spin
		}
		due := -(wait * 10_000_000 / qpcFreq) // negative = relative time, in 100 ns units
		if due >= 0 {
			if !precise {
				return
			}
			continue
		}
		if s.h != 0 {
			procSetWaitableTimer.Call(s.h, uintptr(unsafe.Pointer(&due)), 0, 0, 0, 0)
			WaitForSingleObject(s.h, INFINITE)
		} else {
			time.Sleep(time.Duration(-due * 100))
		}
		if !precise {
			return
		}
	}
}

func (s *Sleeper) Close() {
	CloseHandle(s.h)
	s.h = 0
}
