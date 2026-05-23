package textproc

import "testing"

func TestProcessAppliesVoiceCommands(t *testing.T) {
	processor := NewProcessor(TextConfig{
		AutoPunctuation: true,
		RemoveFillers:   true,
		EnableCommands:  true,
	})

	got := processor.Process("嗯 今天下午三点开会 逗号 请准时参加 换行 谢谢")
	want := "今天下午三点开会，请准时参加\n谢谢。"
	if got != want {
		t.Fatalf("Process() = %q, want %q", got, want)
	}
}

func TestProcessCanDisableCommands(t *testing.T) {
	processor := NewProcessor(TextConfig{
		AutoPunctuation: false,
		RemoveFillers:   false,
		EnableCommands:  false,
	})

	got := processor.Process("换行")
	if got != "换行" {
		t.Fatalf("Process() = %q, want %q", got, "换行")
	}
}
