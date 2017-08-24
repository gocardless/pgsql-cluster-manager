package pgbouncer

import (
	"bytes"
	"html/template"
	"io/ioutil"

	"github.com/gocardless/pgsql-novips/errors"
)

// PGBouncer represents a set of configuration required to manage a PGBouncer instance,
// with a path to a config template that can be rendered.
type PGBouncer struct {
	ConfigFile         string
	ConfigFileTemplate string // template that can be rendered with Host value
}

// GenerateConfig writes new configuration to PGBouncer.ConfigFile
func (b *PGBouncer) GenerateConfig(host string) error {
	var configBuffer bytes.Buffer
	template, err := b.configTemplate()

	if err != nil {
		return err
	}

	err = template.Execute(&configBuffer, struct{ Host string }{host})

	if err != nil {
		return err
	}

	return ioutil.WriteFile(b.ConfigFile, configBuffer.Bytes(), 0644)
}

func (b *PGBouncer) configTemplate() (*template.Template, error) {
	configFile, err := ioutil.ReadFile(b.ConfigFileTemplate)

	if err != nil {
		return nil, errors.NewErrorWithFields(
			"failed to read PGBouncer config template file",
			&map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		)
	}

	return template.Must(template.New("PGBouncerConfig").Parse(string(configFile))), err
}
