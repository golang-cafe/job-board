package template

import (
	"net/http"
	"strings"
	"embed"
	"time"

	stdtemplate "html/template"
	humanize "github.com/dustin/go-humanize"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

type Template struct {
	templates *stdtemplate.Template
	funcMap   stdtemplate.FuncMap
}

func NewTemplate(fs embed.FS) *Template {
	funcMap := stdtemplate.FuncMap{
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
		"jsescape":  stdtemplate.JSEscapeString,
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
		"jobOlderThanMonths": func(monthYearCreated string, monthsAgo int) bool {
			t, err := time.Parse("January 2006", monthYearCreated)
			if err != nil {
				return false
			}
			return t.Before(time.Now().AddDate(0, -monthsAgo, 0))
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
		templates: stdtemplate.Must(stdtemplate.New("stdtmpl").Funcs(funcMap).ParseFS(fs, "static/views/*.html")),
	}
}

func (t *Template) JSEscapeString(s string) string {
	return stdtemplate.JSEscapeString(s)
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
