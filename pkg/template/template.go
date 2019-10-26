package template

import (
	"net/http"

	stdtemplate "html/template"

	customtemplate "github.com/alecthomas/template"
	blackfriday "gopkg.in/russross/blackfriday.v2"
)

type Template struct {
	templates *customtemplate.Template
}

func NewTemplate() *Template {
	return &Template{
		templates: customtemplate.Must(customtemplate.ParseGlob("static/views/*.html")),
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
	return stdtemplate.HTML(blackfriday.Run([]byte(s)))
}
