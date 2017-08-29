package pgbouncer

import (
	"bufio"
	"bytes"
	"errors"
	"html/template"
	"io/ioutil"
	"os"
	"regexp"
	"time"
)

// PGBouncer provides an interface to interact with a local PGBouncer service
type PGBouncer interface {
	Config() (map[string]string, error)
	GenerateConfig(string) error
	Pause() error
	Reload() error
}

// PGBouncer represents a set of configuration required to manage a PGBouncer
// instance, with a path to a config template that can be rendered.
type pgBouncer struct {
	ConfigFile         string
	ConfigFileTemplate string // template that can be rendered with Host value
	PsqlExecutor
}

type errorWithFields struct {
	error
	fields map[string]interface{}
}

func (e errorWithFields) Fields() map[string]interface{} {
	return e.fields
}

// NewPGBouncer returns a PGBouncer configured around the given configFile and template
func NewPGBouncer(configFile, configFileTemplate string, psqlTimeout time.Duration) PGBouncer {
	bouncer := pgBouncer{
		ConfigFile:         configFile,
		ConfigFileTemplate: configFileTemplate,
	}

	bouncer.PsqlExecutor = NewPGBouncerExecutor(&bouncer, psqlTimeout)

	return &bouncer
}

// Config generates a key value map of config parameters from the PGBouncer config
// template file
func (b pgBouncer) Config() (map[string]string, error) {
	config := make(map[string]string)
	configFile, err := os.Open(b.ConfigFileTemplate)

	if err != nil {
		return nil, errorWithFields{
			errors.New("Failed to read PGBouncer config template file"),
			map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		}
	}

	defer configFile.Close()

	r, _ := regexp.Compile("^(\\S+)\\s*\\=\\s*(\\S+)$")
	scanner := bufio.NewScanner(configFile)

	for scanner.Scan() {
		line := scanner.Text()
		if result := r.FindStringSubmatch(line); result != nil {
			config[result[1]] = result[2]
		}
	}

	return config, nil
}

// GenerateConfig writes new configuration to PGBouncer.ConfigFile
func (b pgBouncer) GenerateConfig(host string) error {
	var configBuffer bytes.Buffer
	template, err := b.createTemplate()

	if err != nil {
		return err
	}

	err = template.Execute(&configBuffer, struct{ Host string }{host})

	if err != nil {
		return err
	}

	return ioutil.WriteFile(b.ConfigFile, configBuffer.Bytes(), 0644)
}

func (b pgBouncer) createTemplate() (*template.Template, error) {
	configTemplate, err := ioutil.ReadFile(b.ConfigFileTemplate)

	if err != nil {
		return nil, errorWithFields{
			errors.New("Failed to read PGBouncer config template file"),
			map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		}
	}

	return template.Must(template.New("PGBouncerConfig").Parse(string(configTemplate))), err
}

type fieldError interface {
	Field(byte) string
}

// AlreadyPausedError is the field returned as the error code when PGBouncer is already
// paused, and you issue a PAUSE;
const AlreadyPausedError string = "08P01"

// Pause causes PGBouncer to buffer incoming queries while waiting for those currently
// processing to finish executing. The supplied timeout is applied to the Postgres
// connection.
func (b pgBouncer) Pause() error {
	if _, err := b.PsqlExecutor.Exec(`PAUSE;`); err != nil {
		if ferr, ok := err.(fieldError); ok {
			// We get this when PGBouncer tells us we're already paused
			if ferr.Field('C') == AlreadyPausedError {
				return nil // ignore the error, as the pause was not required
			}
		}

		return errorWithFields{
			errors.New("Failed to pause PGBouncer"),
			map[string]interface{}{
				"error": err.Error(),
			},
		}
	}

	return nil
}

// Reload will cause PGBouncer to reload configuration and live apply setting changes
func (b pgBouncer) Reload() error {
	if _, err := b.PsqlExecutor.Exec(`RELOAD;`); err != nil {
		return errorWithFields{
			errors.New("Failed to reload PGBouncer"),
			map[string]interface{}{
				"error": err.Error(),
			},
		}
	}

	return nil
}
