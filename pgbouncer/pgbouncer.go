package pgbouncer

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"html/template"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/lib/pq"
	"github.com/pkg/errors"
)

type PGBouncer struct {
	ConfigFile         string
	ConfigTemplateFile string // template that can be rendered with Host value
	Executor           executor
}

// Config generates a key value map of config parameters from the PGBouncer config
// template file
func (b *PGBouncer) Config() (map[string]string, error) {
	config := make(map[string]string)
	configFile, err := os.Open(b.ConfigTemplateFile)

	if err != nil {
		return nil, errors.Wrap(err, "failed to read PGBouncer config template file")
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
func (b *PGBouncer) GenerateConfig(host string) error {
	var configBuffer bytes.Buffer
	template, err := b.createTemplate()

	if err != nil {
		return err
	}

	err = template.Execute(&configBuffer, struct{ Host string }{host})

	if err != nil {
		return errors.Wrap(err, "failed to render PGBouncer config")
	}

	return ioutil.WriteFile(b.ConfigFile, configBuffer.Bytes(), 0644)
}

func (b *PGBouncer) createTemplate() (*template.Template, error) {
	configTemplate, err := ioutil.ReadFile(b.ConfigTemplateFile)

	if err != nil {
		return nil, errors.Wrap(err, "failed to read PGBouncer config template file")
	}

	if matched, _ := regexp.Match("ignore_startup_parameters\\s*\\=.+extra_float_digits", configTemplate); !matched {
		return nil, errors.Errorf(
			"PGBouncer is misconfigured: expected config file '%s' to define "+
				"'ignore_startup_paramets' to include 'extra_float_digits'",
			b.ConfigTemplateFile,
		)
	}

	return template.Must(template.New("PGBouncerConfig").Parse(string(configTemplate))), err
}

type Database struct {
	Name, Host, Port string
}

// ShowDatabase extracts information from the SHOW DATABASE PGBouncer command, selecting
// columns about database host details. This is quite cumbersome to write, due to the
// inability to query select fields for database information, and the lack of guarantees
// about the ordering of the columns returned from the command.
func (b *PGBouncer) ShowDatabases(ctx context.Context) ([]Database, error) {
	databases := make([]Database, 0)
	rows, err := b.Executor.QueryContext(ctx, `SHOW DATABASES;`)

	if err != nil {
		return databases, err
	}

	defer rows.Close()

	cols, _ := rows.Columns()
	columnPointers := make([]interface{}, len(cols))

	indexOfColumn := func(c string) int {
		for idx, column := range cols {
			if column == c {
				return idx
			}
		}

		return -1
	}

	var name, host, port, null sql.NullString

	for idx := range columnPointers {
		columnPointers[idx] = &null
	}

	columnPointers[indexOfColumn("name")] = &name
	columnPointers[indexOfColumn("host")] = &host
	columnPointers[indexOfColumn("port")] = &port

	for rows.Next() {
		err := rows.Scan(columnPointers...)

		if err != nil {
			return databases, err
		}

		databases = append(databases, Database{
			name.String, host.String, port.String,
		})
	}

	return databases, rows.Err()
}

// These error codes are returned whenever PGBouncer is asked to PAUSE/RESUME, but is
// already in the given state.
const PoolerError = "08P01"
const AlreadyPausedError = "already suspended/paused"
const AlreadyResumedError = "Pooler is not paused/suspended"

// Pause causes PGBouncer to buffer incoming queries while waiting for those currently
// processing to finish executing. The supplied timeout is applied to the Postgres
// connection.
func (b *PGBouncer) Pause(ctx context.Context) error {
	if _, err := b.Executor.ExecContext(ctx, `PAUSE;`); err != nil {
		if err, ok := err.(*pq.Error); ok {
			if string(err.Code) == PoolerError && err.Message == AlreadyPausedError {
				return nil
			}
		}

		return errors.Wrap(err, "failed to pause PGBouncer")
	}

	return nil
}

// Resume will remove any applied pauses to PGBouncer
func (b *PGBouncer) Resume(ctx context.Context) error {
	if _, err := b.Executor.ExecContext(ctx, `RESUME;`); err != nil {
		if err, ok := err.(*pq.Error); ok {
			if string(err.Code) == PoolerError && err.Message == AlreadyResumedError {
				return nil
			}
		}

		return errors.Wrap(err, "failed to resume PGBouncer")
	}

	return nil
}

// Reload will cause PGBouncer to reload configuration and live apply setting changes
func (b *PGBouncer) Reload(ctx context.Context) error {
	if _, err := b.Executor.ExecContext(ctx, `RELOAD;`); err != nil {
		return errors.Wrap(err, "failed to reload PGBouncer")
	}

	return nil
}
