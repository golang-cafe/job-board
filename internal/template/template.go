package template

import (
	"net/http"
	"strings"
	"time"

	stdtemplate "html/template"

	customtemplate "github.com/alecthomas/template"
	humanize "github.com/dustin/go-humanize"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

type Template struct {
	templates *customtemplate.Template
	funcMap   stdtemplate.FuncMap
}

func NewTemplate() *Template {
	funcMap := customtemplate.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"last": func(a []int) int {
			if len(a) == 0 {
				return -1
			}
			return a[len(a)-1]
		},
		"jsescape":  customtemplate.JSEscapeString,
		"humantime": humanize.Time,
		"humannumber": func(n int) string {
			return humanize.Comma(int64(n))
		},
		"isTimeBeforeNow": func(t time.Time) bool {
			return t.Before(time.Now())
		},
		"isTimeAfterNow": func(t time.Time) bool {
			return t.After(time.Now())
		},
		"truncateName": func(s string) string {
			parts := strings.Split(s, " ")
			return parts[0]
		},
		"stringTitle": func(s string) string {
			return strings.Title(s)
		},
		"replaceDash": func(s string) string {
			return strings.ReplaceAll(s, "-", " ")
		},
		"mul": func(a int, b int) int {
			return a*b
		},
		"currencysymbol": func(currency string) string {
			symbols := map[string]string{
				"USD": "$",
				"EUR": "€",
				"JPY": "¥",
				"GBP": "£",
				"AUD": "A$",
				"CAD": "C$",
				"CHF": "Fr",
				"CNY": "元",
				"HKD": "HK$",
				"NZD": "NZ$",
				"SEK": "kr",
				"KRW": "₩",
				"SGD": "S$",
				"NOK": "kr",
				"MXN": "MX$",
				"INR": "₹",
				"RUB": "₽",
				"ZAR": "R",
				"TRY": "₺",
				"BRL": "R$",
			}
			symbol, ok := symbols[currency]
			if !ok {
				return "$"
			}
			return symbol
		},
	}
	return &Template{
		templates: customtemplate.Must(customtemplate.New("stdtmpl").Funcs(funcMap).ParseGlob("static/views/*.html")),
	}
}

func (t *Template) JSEscapeString(s string) string {
	return customtemplate.JSEscapeString(s)
}

func (t *Template) Render(w http.ResponseWriter, status int, name string, data interface{}) error {
	w.WriteHeader(status)
	return t.templates.ExecuteTemplate(w, name, data)
}

func (t *Template) StringToHTML(s string) stdtemplate.HTML {
	return stdtemplate.HTML(s)
}

func (t *Template) MarkdownToHTML(s string) stdtemplate.HTML {
	renderer := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{
		Flags: blackfriday.Safelink |
			blackfriday.NofollowLinks |
			blackfriday.NoreferrerLinks |
			blackfriday.HrefTargetBlank,
	})
	return stdtemplate.HTML(blackfriday.Run([]byte(s), blackfriday.WithRenderer(renderer)))
}
