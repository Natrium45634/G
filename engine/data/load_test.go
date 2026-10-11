package data

import (
	"testing"
	"testing/fstest"
)

func TestEmbeddedContentLoadsAndValidates(t *testing.T) {
	c, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Classes) < 8 || len(c.Skills) < 15 || len(c.Scenarios) < 2 {
		t.Fatalf("неполный набор данных: классов %d, умений %d, сценариев %d", len(c.Classes), len(c.Skills), len(c.Scenarios))
	}
	if c.Ticks(1) != c.Rules.TicksPerSecond || c.Ticks(0.001) != 1 || c.Ticks(0) != 0 {
		t.Fatal("перевод секунд в тики неверен")
	}
	if AP(0.5) != 500 || APf(1500) != 1.5 {
		t.Fatal("перевод ОД неверен")
	}
}

func TestValidationReportsBrokenReferences(t *testing.T) {
	fsys := fstest.MapFS{}
	if err := ExportEmbedded(func(name string, b []byte) error {
		fsys[name] = &fstest.MapFile{Data: b}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fsys["skills.json"] = &fstest.MapFile{Data: []byte(`[{"id":"x","name":"X","kind":"attack","color":[1,1,1],"ap_cost":1,"targeting":"unit","shape":{"type":"single"},"ops":[{"op":"apply_effect","effect":"no_such_effect"}]}]`)}
	if _, err := Load(fsys); err == nil {
		t.Fatal("ожидалась ошибка валидации ссылок")
	}
	fsys["skills.json"] = &fstest.MapFile{Data: []byte(`[{"id":"x","unknown_field":1}]`)}
	if _, err := Load(fsys); err == nil {
		t.Fatal("ожидалась ошибка неизвестного поля")
	}
}
