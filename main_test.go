package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func mockApp() *cli.App {
	app := cli.NewApp()
	app.Flags = []cli.Flag{cli.StringFlag{Name: "global"}}
	app.Commands = []cli.Command{
		{
			Name:   "command",
			Flags:  []cli.Flag{cli.StringFlag{Name: "local"}},
			Action: checkMissingFlags,
		},
	}

	return app
}

func TestSuccessWithAllFlags(t *testing.T) {
	err := mockApp().Run([]string{"", "--global", "g", "command", "--local", "l"})
	assert.Nil(t, err, "should not return error")
}

func TestErrorWhenMissingGlobal(t *testing.T) {
	err := mockApp().Run([]string{"", "command", "--local", "l"})
	if assert.Error(t, err, "did not return an error") {
		assert.Regexp(t, "Missing configuration flags.*global", err.Error())
	}
}

func TestErrorWhenMissingLocal(t *testing.T) {
	err := mockApp().Run([]string{"", "--global", "g", "command"})
	if assert.Error(t, err, "did not return an error") {
		assert.Regexp(t, "Missing configuration flags.*local", err.Error())
	}
}
