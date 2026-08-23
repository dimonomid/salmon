package setup

import (
	"fmt"
	"strings"
	"text/template"
)

func renderTemplate(name, source string, data interface{}, functions template.FuncMap) (string, error) {
	template, err := template.New(name).Funcs(functions).Parse(source)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var output strings.Builder
	if err := template.Execute(&output, data); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	return output.String(), nil
}
