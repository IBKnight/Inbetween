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
	guiIDStart
)

// RunGUI shows a minimal launcher: pick the game window, the algorithm and the
// multiplier, then runs live mode. Once live mode exits (the game window closed, or
// Ctrl+Alt+Q), the launcher reappears so another game can be picked without relaunching
// the process.
func RunGUI(cfg *config.Config) error {
	if err := win.SetDPIAware(); err != nil {
		log.Printf("warning: DPI awareness: %v", err)
	}
	for {
		target, algo, mult, start, err := showLauncher()
		if err != nil {
			return err
		}
		if !start {
			return nil
		}
		run := *cfg
		run.Window, run.Algo, run.Mult = target, algo, mult
		if err := RunLive(&run); err != nil {
			log.Printf("ОШИБКА: %v", err)
		}
	}
}

// showLauncher displays the launcher window and blocks until the user either starts
// (returns the chosen window/algorithm/multiplier and start=true) or closes it
// (start=false).
func showLauncher() (target, algo string, mult int, start bool, err error) {
	dlg, err := win.NewDialog("Inbetween", 340, 240)
	if err != nil {
		return "", "", 0, false, err
	}
	defer dlg.Destroy()

	dlg.AddStatic("Окно игры:", 12, 12, 300, 16)
	combo := dlg.AddCombo(guiIDCombo, 12, 32, 220, 200)
	dlg.AddButton("Обновить", guiIDRefresh, 242, 31, 86, 24, false)

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

	dlg.AddStatic("Алгоритм:", 12, 70, 80, 18)
	algoOff := dlg.AddRadio("off", guiIDAlgoOff, 100, 68, 55, 20, true)
	algoBlend := dlg.AddRadio("blend", guiIDAlgoBlend, 160, 68, 65, 20, false)
	algoFlow := dlg.AddRadio("flow", guiIDAlgoFlow, 230, 68, 55, 20, false)
	dlg.SetChecked(algoFlow, true)

	dlg.AddStatic("Множитель:", 12, 98, 80, 18)
	multX2 := dlg.AddRadio("X2", guiIDMultX2, 100, 96, 50, 20, true)
	multX3 := dlg.AddRadio("X3", guiIDMultX3, 155, 96, 50, 20, false)
	dlg.SetChecked(multX2, true)

	dlg.AddButton("Старт", guiIDStart, 12, 132, 316, 30, true)
	status := dlg.AddStatic("Выберите окно игры и нажмите «Старт».", 12, 172, 316, 50)

	dlg.OnCommand(func(id int, _ uint16) {
		switch id {
		case guiIDRefresh:
			refresh()
			dlg.SetText(status, "Список окон обновлён.")
		case guiIDStart:
			if len(windows) == 0 {
				dlg.SetText(status, "Нет доступных окон — нажмите «Обновить».")
				return
			}
			idx := dlg.ComboSelected(combo)
			if idx < 0 || idx >= len(windows) {
				dlg.SetText(status, "Выберите окно игры из списка.")
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
			start = true
			dlg.Destroy()
		}
	})

	dlg.Run()
	return target, algo, mult, start, nil
}
