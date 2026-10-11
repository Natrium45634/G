// Command hextactics — настольная игра на тактическом движке.
//
// Запуск: двойной клик по исполняемому файлу. Если рядом с игрой есть папка data
// с JSON-определениями, используются они (модификации); иначе — встроенные данные.
//
// Флаги (необязательно):
//
//	-scenario id   сразу открыть сценарий (training, river_crossing)
//	-export-data   выгрузить встроенные данные в папку data для правки и выйти
//	-screenshot f  сохранить PNG кадра и выйти (для автоматической проверки)
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"hextactics/engine/data"
	"hextactics/ui"
)

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	_ = os.WriteFile(filepath.Join(exeDir(), "hextactics-error.txt"), []byte(msg), 0o644)
	os.Exit(1)
}

func main() {
	scenario := flag.String("scenario", "", "сценарий для немедленного запуска")
	export := flag.Bool("export-data", false, "выгрузить встроенные данные в папку data")
	shot := flag.String("screenshot", "", "сохранить кадр в PNG и выйти")
	shotFrame := flag.Int("screenshot-frame", 30, "номер кадра для снимка")
	setup := flag.String("setup", "", "отладка: подготовка сцены для снимка (plan, combat)")
	flag.Parse()

	dir := exeDir()
	dataDir := filepath.Join(dir, "data")
	if *export {
		err := data.ExportEmbedded(func(name string, b []byte) error {
			p := filepath.Join(dataDir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			return os.WriteFile(p, b, 0o644)
		})
		if err != nil {
			fatal("Ошибка выгрузки данных: " + err.Error())
		}
		fmt.Println("Данные выгружены в", dataDir)
		return
	}

	var fsys fs.FS = data.Embedded()
	source := "встроенные"
	if st, err := os.Stat(dataDir); err == nil && st.IsDir() {
		fsys = os.DirFS(dataDir)
		source = "папка data (модификация)"
	}
	content, err := data.Load(fsys)
	if err != nil {
		fatal("Не удалось загрузить данные игры (" + source + "):\n" + err.Error())
	}

	g := ui.NewGame(content, source, filepath.Join(dir, "quicksave.json"))
	if *scenario != "" {
		if err := g.StartBattle(*scenario); err != nil {
			fatal(err.Error())
		}
		if *setup != "" {
			ui.DebugSetup(g, *setup)
		}
	}
	if *shot != "" {
		target := *shotFrame
		g.Screenshot = func(img *ebiten.Image) {
			if g.Frame < target {
				return
			}
			f, err := os.Create(*shot)
			if err == nil {
				_ = png.Encode(f, img)
				f.Close()
			}
			os.Exit(0)
		}
	}

	ebiten.SetWindowTitle("Hex Tactics — тактический движок")
	ebiten.SetWindowIcon([]image.Image{ui.WindowIcon(16), ui.WindowIcon(32), ui.WindowIcon(48), ui.WindowIcon(64)})
	ebiten.SetWindowSize(1600, 900)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)
	if err := ebiten.RunGame(g); err != nil && !errors.Is(err, ui.ErrQuit) {
		fatal("Ошибка: " + err.Error())
	}
}
