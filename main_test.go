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

func TestCheckMissingFlags_SuccessWithAllFlags(t *testing.T) {
	err := mockApp().Run([]string{"", "--global", "g", "command", "--local", "l"})
	assert.Nil(t, err, "should not return error")
}

func TestCheckMissingFlags_ErrorWhenMissingGlobal(t *testing.T) {
	err := mockApp().Run([]string{"", "command", "--local", "l"})
	if assert.Error(t, err, "did not return an error") {
		assert.Regexp(t, "Missing configuration flags.*global", err.Error())
	}
}

func TestCheckMissingFlags_ErrorWhenMissingLocal(t *testing.T) {
	err := mockApp().Run([]string{"", "--global", "g", "command"})
	if assert.Error(t, err, "did not return an error") {
		assert.Regexp(t, "Missing configuration flags.*local", err.Error())
	}
}
