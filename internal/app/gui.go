//go:build windows

package app

import (
	"log"

	"inbetween/internal/config"
	"inbetween/internal/win"
)

// Control ids for the launcher dialog.
const (
	guiIDCombo = 100 + iota
	guiIDRefresh
	guiIDAlgoOff
	guiIDAlgoBlend
	guiIDAlgoFlow
	guiIDMultX2
	guiIDMultX3
	guiIDNoVSync
	guiIDSmoother
	guiIDStart
	guiIDLang
)

// uiLang is the launcher's display language. Algorithm names (off/blend/flow) and
// multipliers (X2/X3) are never translated — they're identifiers, not prose.
type uiLang int

const (
	langRU uiLang = iota
	langEN
)

// uiStrings holds every translatable label in the launcher. langToggle is the OTHER
// language's name — it's what the toggle button shows (click it to switch TO that language).
type uiStrings struct {
	windowLabel, refresh, algoLabel, multLabel string
	noVSync, smoother, start, langToggle       string
	idle, refreshed, noWindows, pickWindow     string
}

var uiText = map[uiLang]uiStrings{
	langRU: {
		windowLabel: "Окно игры:",
		refresh:     "Обновить",
		algoLabel:   "Алгоритм:",
		multLabel:   "Множитель:",
		noVSync:     "Без VSync (если дёргается на обычном мониторе без G-Sync/FreeSync)",
		smoother:    "Сильнее сглаживать тайминг показов (+неск. мс задержки)",
		start:       "Старт",
		langToggle:  "EN",
		idle:        "Выберите окно игры и нажмите «Старт».",
		refreshed:   "Список окон обновлён.",
		noWindows:   "Нет доступных окон — нажмите «Обновить».",
		pickWindow:  "Выберите окно игры из списка.",
	},
	langEN: {
		windowLabel: "Game window:",
		refresh:     "Refresh",
		algoLabel:   "Algorithm:",
		multLabel:   "Multiplier:",
		noVSync:     "Disable VSync (if stuttering on a regular monitor without G-Sync/FreeSync)",
		smoother:    "Smooth present timing harder (+ a few ms of latency)",
		start:       "Start",
		langToggle:  "RU",
		idle:        `Pick a game window and press "Start".`,
		refreshed:   "Window list refreshed.",
		noWindows:   `No windows available — press "Refresh".`,
		pickWindow:  "Pick a game window from the list.",
	},
}

// RunGUI shows a minimal launcher: pick the game window, the algorithm and the
// multiplier, then runs live mode. Once live mode exits (the game window closed, or
// Ctrl+Alt+Q), the launcher reappears so another game can be picked without relaunching
// the process.
func RunGUI(cfg *config.Config) error {
	if err := win.SetDPIAware(); err != nil {
		log.Printf("warning: DPI awareness: %v", err)
	}
	for {
		target, algo, mult, noVSync, smoother, start, err := showLauncher()
		if err != nil {
			return err
		}
		if !start {
			return nil
		}
		run := *cfg
		run.Window, run.Algo, run.Mult = target, algo, mult
		// The launcher isn't a shader-editing workflow: hot reload only adds an mtime
		// stat() poll every ~0.5s per shader/include, and a risk of a false-positive
		// touch (AV scan, cloud sync) triggering a synchronous recompile mid-session.
		run.HotReload = false
		if noVSync {
			run.VSync, run.Tearing = false, true
		}
		if smoother {
			run.Offset = 0.3
		}
		if err := RunLive(&run); err != nil {
			log.Printf("ОШИБКА: %v", err)
		}
	}
}

// showLauncher displays the launcher window and blocks until the user either starts
// (returns the chosen window/algorithm/multiplier/vsync-off/smoother and start=true) or
// closes it (start=false).
func showLauncher() (target, algo string, mult int, noVSync, smoother, start bool, err error) {
	dlg, err := win.NewDialog("Inbetween", 340, 312)
	if err != nil {
		return "", "", 0, false, false, false, err
	}
	defer dlg.Destroy()

	cur := langRU
	t := uiText[cur]

	windowLabel := dlg.AddStatic(t.windowLabel, 12, 12, 160, 16)
	langBtn := dlg.AddButton(t.langToggle, guiIDLang, 270, 10, 58, 20, false)
	combo := dlg.AddCombo(guiIDCombo, 12, 32, 220, 200)
	refreshBtn := dlg.AddButton(t.refresh, guiIDRefresh, 242, 31, 86, 24, false)

	var windows []win.WindowInfo
	refresh := func() {
		windows = win.ListWindows()
		dlg.ComboReset(combo)
		for _, w := range windows {
			dlg.ComboAdd(combo, w.Title)
		}
		if len(windows) > 0 {
			dlg.ComboSelect(combo, 0)
		}
	}
	refresh()

	algoLabel := dlg.AddStatic(t.algoLabel, 12, 70, 80, 18)
	algoOff := dlg.AddRadio("off", guiIDAlgoOff, 100, 68, 55, 20, true)
	algoBlend := dlg.AddRadio("blend", guiIDAlgoBlend, 160, 68, 65, 20, false)
	algoFlow := dlg.AddRadio("flow", guiIDAlgoFlow, 230, 68, 55, 20, false)
	dlg.SetChecked(algoFlow, true)

	multLabel := dlg.AddStatic(t.multLabel, 12, 98, 80, 18)
	multX2 := dlg.AddRadio("X2", guiIDMultX2, 100, 96, 50, 20, true)
	multX3 := dlg.AddRadio("X3", guiIDMultX3, 155, 96, 50, 20, false)
	dlg.SetChecked(multX2, true)

	// Off by default: only useful without VRR, and trades a bit of tearing for presents
	// that aren't gated on the display's fixed vblank slots.
	noVSyncBox := dlg.AddCheckbox(t.noVSync, guiIDNoVSync, 12, 120, 316, 36)
	// Off by default: shifts the whole present schedule later (pacing.Pacer.Offset), giving
	// slack against uneven source-frame arrival at the cost of a few ms of extra latency.
	smootherBox := dlg.AddCheckbox(t.smoother, guiIDSmoother, 12, 158, 316, 36)

	startBtn := dlg.AddButton(t.start, guiIDStart, 12, 204, 316, 30, true)
	status := dlg.AddStatic(t.idle, 12, 244, 316, 50)

	statusKey := "idle"
	setStatus := func(key string) {
		statusKey = key
		s := uiText[cur]
		switch key {
		case "refreshed":
			dlg.SetText(status, s.refreshed)
		case "nowindows":
			dlg.SetText(status, s.noWindows)
		case "pickwindow":
			dlg.SetText(status, s.pickWindow)
		default:
			dlg.SetText(status, s.idle)
		}
	}

	dlg.OnCommand(func(id int, _ uint16) {
		switch id {
		case guiIDLang:
			if cur == langRU {
				cur = langEN
			} else {
				cur = langRU
			}
			s := uiText[cur]
			dlg.SetText(windowLabel, s.windowLabel)
			dlg.SetText(langBtn, s.langToggle)
			dlg.SetText(refreshBtn, s.refresh)
			dlg.SetText(algoLabel, s.algoLabel)
			dlg.SetText(multLabel, s.multLabel)
			dlg.SetText(noVSyncBox, s.noVSync)
			dlg.SetText(smootherBox, s.smoother)
			dlg.SetText(startBtn, s.start)
			setStatus(statusKey)
		case guiIDRefresh:
			refresh()
			setStatus("refreshed")
		case guiIDStart:
			if len(windows) == 0 {
				setStatus("nowindows")
				return
			}
			idx := dlg.ComboSelected(combo)
			if idx < 0 || idx >= len(windows) {
				setStatus("pickwindow")
				return
			}
			target = windows[idx].Title
			switch {
			case dlg.IsChecked(algoOff):
				algo = "off"
			case dlg.IsChecked(algoBlend):
				algo = "blend"
			default:
				algo = "flow"
			}
			mult = 2
			if dlg.IsChecked(multX3) {
				mult = 3
			}
			noVSync = dlg.IsChecked(noVSyncBox)
			smoother = dlg.IsChecked(smootherBox)
			start = true
			dlg.Destroy()
		}
	})

	dlg.Run()
	return target, algo, mult, noVSync, smoother, start, nil
}
