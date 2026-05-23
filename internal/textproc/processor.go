package textproc

import (
	"regexp"
	"strings"
)

type TextConfig struct {
	AutoPunctuation bool     `json:"autoPunctuation"`
	RemoveFillers   bool     `json:"removeFillers"`
	EnableCommands  bool     `json:"enableCommands"`
	Hotwords        []string `json:"hotwords"`
}

type Processor struct {
	config TextConfig
}

func NewProcessor(config TextConfig) *Processor {
	return &Processor{config: config}
}

func (p *Processor) UpdateConfig(config TextConfig) {
	p.config = config
}

func (p *Processor) Process(input string) string {
	text := strings.TrimSpace(input)
	text = normalizeSpaces(text)

	if p.config.RemoveFillers {
		text = removeFillers(text)
	}

	if p.config.EnableCommands {
		text = applyVoiceCommands(text)
	}

	text = tidyPunctuationSpacing(text)

	if p.config.AutoPunctuation {
		text = ensureSentenceEnd(text)
	}

	return strings.TrimSpace(text)
}

func normalizeSpaces(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\t", " ")
	return strings.Join(strings.Fields(text), " ")
}

func removeFillers(text string) string {
	replacer := strings.NewReplacer("嗯", "", "呃", "", "啊", "")
	return replacer.Replace(text)
}

func applyVoiceCommands(text string) string {
	replacer := strings.NewReplacer(
		"换行", "\n",
		"新的一行", "\n",
		"逗号", "，",
		"句号", "。",
		"问号", "？",
		"感叹号", "！",
		"空格", " ",
	)
	return replacer.Replace(text)
}

func tidyPunctuationSpacing(text string) string {
	replacer := strings.NewReplacer(
		" ， ", "，",
		" 。 ", "。",
		" ？ ", "？",
		" ！ ", "！",
		" ，", "，",
		" 。", "。",
		" ？", "？",
		" ！", "！",
		"， ", "，",
		"。 ", "。",
		"？ ", "？",
		"！ ", "！",
		" \n ", "\n",
		" \n", "\n",
		"\n ", "\n",
	)
	return replacer.Replace(text)
}

func ensureSentenceEnd(text string) string {
	if text == "" {
		return text
	}
	last := []rune(text)[len([]rune(text))-1]
	if regexp.MustCompile(`[。！？.!?,，\n]`).MatchString(string(last)) {
		return text
	}
	return text + "。"
}
